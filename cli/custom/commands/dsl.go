package commands

import (
	"errors"
	"fmt"
	"os"
	"time"

	"orq/cli/custom/dsl"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
)

// NewDSLCommand groups the declarative provisioning commands:
// orq dsl validate|plan|apply|pull|destroy|state|init.
func NewDSLCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dsl",
		Short: "Declarative workspace provisioning (validate, plan, apply, pull, destroy)",
		Long: "Describe orq workspace resources as YAML manifests and reconcile them\n" +
			"against the live workspace: plan shows the diff, apply executes it,\n" +
			"pull serializes live resources back to files.",
	}
	cmd.AddCommand(
		newDSLInitCommand(),
		newDSLValidateCommand(),
		newDSLPlanCommand(),
		newDSLApplyCommand(),
		newDSLPullCommand(),
		newDSLDestroyCommand(),
		newDSLStateCommand(),
	)
	return cmd
}

// addStackFlags wires the flags shared by every command that reads a stack
// directory. Returned pointers stay valid for the command's RunE closure.
func addStackFlags(cmd *cobra.Command) (dir *string, varFile *string, cliVars *[]string) {
	dir = cmd.Flags().StringP("file", "f", ".", "Stack directory (containing orq.yaml)")
	varFile = cmd.Flags().String("var-file", "", "YAML file with variable values")
	cliVars = cmd.Flags().StringArray("var", nil, "Variable override name=value (repeatable)")
	return dir, varFile, cliVars
}

func newDSLInitCommand() *cobra.Command {
	var stack, dir string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold orq.yaml and an example manifest",
		RunE: func(cmd *cobra.Command, args []string) error {
			files, err := dsl.Init(dir, stack)
			if err != nil {
				return err
			}
			for _, f := range files {
				fmt.Fprintf(cmd.OutOrStdout(), "created  %s\n", f)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\nnext:\n  orq dsl validate -f %s\n  orq dsl plan -f %s\n", dir, dir)
			return nil
		},
	}
	cmd.Flags().StringVar(&stack, "stack", "", "Stack name (default: directory name)")
	cmd.Flags().StringVarP(&dir, "file", "f", ".", "Directory to scaffold")
	return cmd
}

func newDSLValidateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate manifests offline (schema, refs, vars) — no credentials needed",
	}
	dir, varFile, cliVars := addStackFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		ms, _, errs := dsl.Validate(*dir, *varFile, *cliVars)
		if len(errs) > 0 {
			for _, e := range errs {
				fmt.Fprintf(cmd.ErrOrStderr(), "✗ %s\n", e.Error())
			}
			return fmt.Errorf("%d validation error(s)", len(errs))
		}
		kinds := map[string]bool{}
		for _, m := range ms {
			kinds[m.Kind] = true
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✓ %d manifests · %d kinds · schema ok · refs ok · vars ok\n", len(ms), len(kinds))
		return nil
	}
	return cmd
}

func newDSLPlanCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Show the changes apply would make (exit 2 when changes are pending)",
	}
	dir, varFile, cliVars := addStackFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		plan, err := buildPlanFromFlags(cmd, *dir, *varFile, *cliVars)
		if err != nil {
			return err
		}
		dsl.RenderPlan(cmd.OutOrStdout(), plan, true)
		if plan.HasChanges() {
			fmt.Fprintf(cmd.OutOrStdout(), "\nRun `orq dsl apply -f %s` to execute.\n", *dir)
			os.Exit(2)
		}
		return nil
	}
	return cmd
}

// buildPlanFromFlags: shared validate→state→plan pipeline for plan/apply/destroy.
func buildPlanFromFlags(cmd *cobra.Command, dir, varFile string, cliVars []string) (*dsl.PlanResult, error) {
	ms, cfg, verrs := dsl.Validate(dir, varFile, cliVars)
	if len(verrs) > 0 {
		for _, e := range verrs {
			fmt.Fprintf(cmd.ErrOrStderr(), "✗ %s\n", e.Error())
		}
		return nil, fmt.Errorf("%d validation error(s)", len(verrs))
	}
	client := dsl.NewClient()
	st, stateID, err := dsl.LoadState(client, cfg.Stack)
	if err != nil {
		return nil, err
	}
	plan, err := dsl.BuildPlan(ms, cfg, client, st, stateID)
	if err != nil {
		return nil, err
	}
	plan.Config = cfg
	return plan, nil
}

func newDSLApplyCommand() *cobra.Command {
	var autoApprove bool
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Reconcile the workspace to match the manifests",
	}
	dir, varFile, cliVars := addStackFlags(cmd)
	cmd.Flags().BoolVar(&autoApprove, "auto-approve", false, "Skip the confirmation prompt")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		plan, err := buildPlanFromFlags(cmd, *dir, *varFile, *cliVars)
		if err != nil {
			return err
		}
		dsl.RenderPlan(cmd.OutOrStdout(), plan, true)
		if !plan.HasChanges() {
			return nil
		}
		total := plan.Creates + plan.Updates + plan.Deletes + plan.Replaces
		if !autoApprove {
			ok := false
			prompt := &survey.Confirm{Message: fmt.Sprintf("Apply these %d changes?", total)}
			if err := survey.AskOne(prompt, &ok); err != nil || !ok {
				return errors.New("apply cancelled")
			}
		}
		fmt.Fprintln(cmd.OutOrStdout())
		if err := dsl.Execute(dsl.NewClient(), plan, cmd.OutOrStdout(), time.Now); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "\nApply complete: %d created, %d updated, %d deleted, %d replaced.\n",
			plan.Creates, plan.Updates, plan.Deletes, plan.Replaces)
		return nil
	}
	return cmd
}

func newDSLPullCommand() *cobra.Command {
	var project, outDir, stack string
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Serialize live workspace resources into manifest files",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := dsl.NewClient()
			var st *dsl.StateDoc
			if stack != "" {
				var err error
				st, _, err = dsl.LoadState(client, stack)
				if err != nil {
					return err
				}
			}
			defaultPath := project
			if defaultPath == "" {
				defaultPath = "Default"
			}
			report, err := dsl.Pull(client, project, outDir, st, defaultPath)
			if err != nil {
				return err
			}
			for _, f := range report.Written {
				fmt.Fprintf(cmd.OutOrStdout(), "written  %s\n", f)
			}
			for _, w := range report.Warnings {
				fmt.Fprintf(cmd.OutOrStdout(), "⚠ %s\n", w)
			}
			for _, s := range report.Skipped {
				fmt.Fprintf(cmd.OutOrStdout(), "skipped  %s (outside --project scope)\n", s)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\npulled %d resources → %s\n", len(report.Written), outDir)
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "Project to pull (name)")
	// no -o shorthand: bartolo's root command owns -o (output format)
	cmd.Flags().StringVar(&outDir, "out", ".", "Output directory")
	cmd.Flags().StringVar(&stack, "stack", "", "Stack whose state should inform paths/identities")
	return cmd
}

func newDSLDestroyCommand() *cobra.Command {
	var autoApprove bool
	cmd := &cobra.Command{
		Use:   "destroy",
		Short: "Delete every resource owned by the stack (reverse dependency order)",
	}
	dir, varFile, cliVars := addStackFlags(cmd)
	_ = varFile
	_ = cliVars
	cmd.Flags().BoolVar(&autoApprove, "auto-approve", false, "Skip the confirmation prompt")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		cfg, err := dsl.LoadStack(*dir)
		if err != nil {
			return err
		}
		client := dsl.NewClient()
		st, stateID, err := dsl.LoadState(client, cfg.Stack)
		if err != nil {
			return err
		}
		if st == nil || len(st.Resources) == 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "Stack %s owns nothing. Nothing to destroy.\n", cfg.Stack)
			return nil
		}
		plan, err := dsl.DestroyPlan(cfg, st, stateID)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Stack %s owns %d resources:\n", cfg.Stack, len(st.Resources))
		for _, r := range st.Resources {
			fmt.Fprintf(cmd.OutOrStdout(), "  − %s\n", r.Identity)
		}
		if !autoApprove {
			var typed string
			prompt := &survey.Input{Message: "Type the stack name to confirm destruction:"}
			if err := survey.AskOne(prompt, &typed); err != nil || typed != cfg.Stack {
				return errors.New("destroy cancelled (stack name mismatch)")
			}
		}
		fmt.Fprintln(cmd.OutOrStdout())
		execErr := dsl.Execute(client, plan, cmd.OutOrStdout(), time.Now)
		if execErr != nil {
			return execErr
		}
		if err := dsl.DeleteState(client, plan.StateID); err != nil {
			return fmt.Errorf("resources destroyed but state skill removal failed: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "\nDestroyed %d resources. Stack state removed.\n", plan.Deletes)
		return nil
	}
	return cmd
}

func newDSLStateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "state",
		Short: "Inspect stack state",
	}
	list := &cobra.Command{
		Use:   "list",
		Short: "Print the stack inventory",
	}
	dir, _, _ := addStackFlags(list)
	list.RunE = func(cmd *cobra.Command, args []string) error {
		cfg, err := dsl.LoadStack(*dir)
		if err != nil {
			return err
		}
		st, _, err := dsl.LoadState(dsl.NewClient(), cfg.Stack)
		if err != nil {
			return err
		}
		if st == nil {
			fmt.Fprintf(cmd.OutOrStdout(), "Stack %s has never been applied (no state).\n", cfg.Stack)
			return nil
		}
		return emit(st)
	}
	cmd.AddCommand(list)
	return cmd
}
