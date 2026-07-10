package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"orq/cli/custom/dsl"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
)

// NewDSLCommand groups the declarative provisioning commands:
// orq stack validate|plan|apply|pull|destroy|state|init ("dsl" kept as alias).
func NewDSLCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "stack",
		Aliases: []string{"dsl"},
		Short:   "Declarative workspace provisioning — validate, plan, apply, pull, destroy",
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
	silenceUsageOnRun(cmd)
	return cmd
}

// silenceUsageOnRun keeps cobra's usage dump for flag/arg mistakes (pre-RunE)
// but suppresses it for runtime failures — a failed validate or API call is
// not a reason to reprint the flag table.
func silenceUsageOnRun(cmd *cobra.Command) {
	for _, c := range cmd.Commands() {
		silenceUsageOnRun(c)
	}
	if run := cmd.RunE; run != nil {
		cmd.RunE = func(c *cobra.Command, args []string) error {
			c.SilenceUsage = true
			return run(c, args)
		}
	}
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
	var stack, dir, project string
	cmd := &cobra.Command{
		Use:   "init [stack]",
		Short: "Scaffold a stack directory with orq.yaml and an example manifest",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && stack == "" {
				stack = args[0]
			}
			// `orq stack init foo` scaffolds ./foo (like git/cargo); an
			// explicit -f wins.
			if stack != "" && !cmd.Flags().Changed("file") {
				dir = stack
			}
			if project == "" {
				def := stack
				if def == "" {
					if abs, err := filepath.Abs(dir); err == nil {
						def = strings.ToLower(filepath.Base(abs))
					}
				}
				prompt := &survey.Input{
					Message: "orq project resources live in (must already exist; the one your API key is scoped to):",
					Default: def,
				}
				// Non-interactive (CI, piped): fall back to the default silently.
				if err := survey.AskOne(prompt, &project); err != nil {
					project = def
				}
			}
			files, err := dsl.Init(dir, stack, project)
			if err != nil {
				return err
			}
			for _, f := range files {
				fmt.Fprintf(cmd.OutOrStdout(), "created  %s\n", f)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\nnext:\n  orq stack validate -f %s\n  orq stack plan -f %s\n", dir, dir)
			return nil
		},
	}
	cmd.Flags().StringVar(&stack, "stack", "", "Stack name (default: directory name)")
	cmd.Flags().StringVar(&project, "project", "", "Existing orq project resources live in (default: stack name)")
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
			dsl.RenderValidationErrors(cmd.ErrOrStderr(), errs, true)
			os.Exit(1)
		}
		kinds := map[string]bool{}
		for _, m := range ms {
			kinds[m.Kind] = true
		}
		dsl.RenderValidateOK(cmd.OutOrStdout(), len(ms), len(kinds), true)
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
			fmt.Fprintf(cmd.OutOrStdout(), "\nRun `orq stack apply -f %s` to execute.\n", *dir)
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
		dsl.RenderValidationErrors(cmd.ErrOrStderr(), verrs, true)
		os.Exit(1)
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
		if len(plan.Adoptions) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "%d unchanged live resource(s) will be adopted into stack state.\n", len(plan.Adoptions))
		}
		if !plan.HasChanges() && len(plan.Adoptions) == 0 {
			return nil
		}
		total := plan.Creates + plan.Updates + plan.Deletes + plan.Replaces
		if !autoApprove && total > 0 {
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
	var all bool
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Serialize live workspace resources into manifest files",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Scope resolution: --project > the enclosing stack's project
			// (orq.yaml defaults.path) > explicit --all. A bare pull in a
			// random directory must not slurp the whole workspace.
			if project == "" && !all {
				if cfg, err := dsl.LoadStack("."); err == nil && cfg.Defaults.Path != "" {
					project, _, _ = strings.Cut(cfg.Defaults.Path, "/")
					fmt.Fprintf(cmd.OutOrStdout(), "scoped to project %q (from orq.yaml) — use --project or --all to override\n", project)
				} else {
					return errors.New("no project scope: pass --project <name>, run inside a stack directory (orq.yaml), or pass --all for the entire workspace")
				}
			}
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
	cmd.Flags().BoolVar(&all, "all", false, "Pull the entire workspace (no project scope)")
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
