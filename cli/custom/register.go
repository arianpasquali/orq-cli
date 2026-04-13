package custom

import (
	"orq/cli/custom/auth"
	"orq/cli/custom/commands"

	bartolocli "github.com/orq-ai/bartolo/cli"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	gentlemanctx "gopkg.in/h2non/gentleman.v2/context"
)

// Register wires custom commands and the session-aware auth middleware onto the
// provided root command. Must be called after generated.Register so that the
// bartolo `auth` parent command exists for our subcommands to attach onto.
func Register(root *cobra.Command) {
	if root == nil {
		root = bartolocli.Root
	}
	installSessionPreRun()
	installSessionMiddleware()
	registerCommands(root)
}

// installSessionPreRun runs once per command invocation, after cobra parses
// flags and before the command handler fires. When the active profile's
// session has an apiBaseUrl set and the user did NOT pass --server explicitly,
// we point bartolo's generated commands at the same host the session was
// authenticated against. This keeps "login against local → query against
// local" working without a separate --server flag on every call.
func installSessionPreRun() {
	prev := bartolocli.PreRun
	bartolocli.PreRun = func(cmd *cobra.Command, args []string) error {
		if prev != nil {
			if err := prev(cmd, args); err != nil {
				return err
			}
		}
		if viper.GetString("server") != "" {
			return nil
		}
		session, err := auth.ReadSession()
		if err != nil || session == nil || session.APIBaseURL == "" {
			return nil
		}
		viper.Set("server", session.APIBaseURL)
		return nil
	}
}

// installSessionMiddleware adds a gentleman request middleware that injects the
// active workspace bearer token from `~/.orq/session.json` when one is
// available. Runs after the bartolo apikey middleware, so it overrides the
// Authorization header set there. Falls back silently to the apikey flow when
// no session exists.
func installSessionMiddleware() {
	bartolocli.Client.UseRequest(func(ctx *gentlemanctx.Context, h gentlemanctx.Handler) {
		if token := activeWorkspaceToken(); token != "" {
			ctx.Request.Header.Set("Authorization", "Bearer "+token)
		}
		h.Next(ctx)
	})
}

func activeWorkspaceToken() string {
	session, err := auth.ReadSession()
	if err != nil || session == nil {
		return ""
	}
	client := auth.NewClient(session.APIBaseURL)
	active, err := client.GetActiveWorkspaceAccessToken()
	if err != nil {
		return ""
	}
	return active.AccessToken
}

func registerCommands(root *cobra.Command) {
	replaceDoctor(root)
	attachAuthSubcommands(root)
	addHiddenAuthAliases(root)
	root.AddCommand(commands.NewWorkspaceCommand())
}

func replaceDoctor(root *cobra.Command) {
	for _, c := range root.Commands() {
		if c.Name() == "doctor" {
			root.RemoveCommand(c)
			break
		}
	}
	root.AddCommand(commands.NewDoctorCommand())
}

func attachAuthSubcommands(root *cobra.Command) {
	var authParent *cobra.Command
	for _, c := range root.Commands() {
		if c.Name() == "auth" {
			authParent = c
			break
		}
	}
	if authParent == nil {
		authParent = &cobra.Command{
			Use:   "auth",
			Short: "Authentication settings",
		}
		root.AddCommand(authParent)
	}
	// Bartolo's `auth setup` command ships with a `login` alias for the
	// API-key wizard. Strip it so our OAuth `auth login` subcommand is the
	// one cobra resolves.
	for _, c := range authParent.Commands() {
		if c.Name() == "setup" {
			c.Aliases = removeString(c.Aliases, "login")
		}
	}
	authParent.AddCommand(commands.NewLoginCommand())
	authParent.AddCommand(commands.NewLogoutCommand())
	authParent.AddCommand(commands.NewWhoAmICommand())
}

func removeString(slice []string, target string) []string {
	out := slice[:0]
	for _, s := range slice {
		if s != target {
			out = append(out, s)
		}
	}
	return out
}

func addHiddenAuthAliases(root *cobra.Command) {
	for _, factory := range []func() *cobra.Command{
		commands.NewLoginCommand,
		commands.NewLogoutCommand,
		commands.NewWhoAmICommand,
	} {
		alias := factory()
		alias.Hidden = true
		root.AddCommand(alias)
	}
}
