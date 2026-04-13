package custom

import "github.com/spf13/cobra"

// Register is the user-owned extension point for custom commands and hooks.
func Register(root *cobra.Command) {
	_ = root
	// Add your own commands and middleware registrations here.
}
