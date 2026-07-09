package commands

import (
	"errors"
	"fmt"

	"orq/cli/custom/dsl"

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

var errNotImplemented = errors.New("not implemented yet")

// addStackFlags wires the flags shared by every command that reads a stack
// directory. Returned pointers stay valid for the command's RunE closure.
func addStackFlags(cmd *cobra.Command) (dir *string, varFile *string, cliVars *[]string) {
	dir = cmd.Flags().StringP("file", "f", ".", "Stack directory (containing orq.yaml)")
	varFile = cmd.Flags().String("var-file", "", "YAML file with variable values")
	cliVars = cmd.Flags().StringArray("var", nil, "Variable override name=value (repeatable)")
	return dir, varFile, cliVars
}

func newDSLInitCommand() *cobra.Command {
	var stack string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold orq.yaml and an example manifest",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errNotImplemented
		},
	}
	cmd.Flags().StringVar(&stack, "stack", "", "Stack name (default: directory name)")
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
		RunE: func(cmd *cobra.Command, args []string) error {
			return errNotImplemented
		},
	}
	addStackFlags(cmd)
	return cmd
}

func newDSLApplyCommand() *cobra.Command {
	var autoApprove bool
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Reconcile the workspace to match the manifests",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errNotImplemented
		},
	}
	addStackFlags(cmd)
	cmd.Flags().BoolVar(&autoApprove, "auto-approve", false, "Skip the confirmation prompt")
	return cmd
}

func newDSLPullCommand() *cobra.Command {
	var project, outDir string
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Serialize live workspace resources into manifest files",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errNotImplemented
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "Project to pull (name)")
	cmd.Flags().StringVarP(&outDir, "out", "o", ".", "Output directory")
	return cmd
}

func newDSLDestroyCommand() *cobra.Command {
	var autoApprove bool
	cmd := &cobra.Command{
		Use:   "destroy",
		Short: "Delete every resource owned by the stack (reverse dependency order)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errNotImplemented
		},
	}
	addStackFlags(cmd)
	cmd.Flags().BoolVar(&autoApprove, "auto-approve", false, "Skip the confirmation prompt")
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
		RunE: func(cmd *cobra.Command, args []string) error {
			return errNotImplemented
		},
	}
	addStackFlags(list)
	cmd.AddCommand(list)
	return cmd
}
