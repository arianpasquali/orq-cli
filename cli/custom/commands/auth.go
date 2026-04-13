package commands

import (
	"errors"
	"fmt"

	"orq/cli/custom/auth"

	survey "github.com/AlecAivazis/survey/v2"
	bartolocli "github.com/orq-ai/bartolo/cli"
	"github.com/spf13/cobra"
)

func NewLoginCommand() *cobra.Command {
	var apiBase string
	var workspace string
	var noOpen bool

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with orq via OAuth device login",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := auth.NewClient(apiBase)

			start, err := client.StartDeviceLogin("orq-cli")
			if err != nil {
				return err
			}

			fmt.Fprintf(bartolocli.Stderr, "Open: %s\n", start.VerificationURIComplete)
			fmt.Fprintf(bartolocli.Stderr, "Code: %s\n", start.UserCode)

			browserOpened := false
			if !noOpen {
				browserOpened = auth.OpenBrowser(start.VerificationURIComplete)
				if !browserOpened {
					fmt.Fprintln(bartolocli.Stderr, "Could not open the browser automatically. Open the URL manually.")
				}
			}

			fmt.Fprintln(bartolocli.Stderr, "Waiting for browser approval...")
			approved, err := client.AwaitDeviceApproval(start.DeviceCode, start.ExpiresIn, start.Interval)
			if err != nil {
				return err
			}

			profile, err := client.FetchProfile(approved.AccessToken)
			if err != nil {
				return err
			}

			workspaceKey := workspace
			if workspaceKey == "" && len(profile.Workspaces) > 1 && hasInteractiveTTY() {
				workspaceKey, err = selectWorkspace(profile.Workspaces, "Choose an active workspace")
				if err != nil {
					return err
				}
			}

			session, err := client.CreateSessionFromDeviceApproval(approved, profile, workspaceKey)
			if err != nil {
				return err
			}

			report := BuildIdentityReport(session, &client.URLs)
			return emit(map[string]any{
				"identity":         report,
				"browser_opened":   browserOpened,
				"verification_uri": start.VerificationURIComplete,
				"user_code":        start.UserCode,
			})
		},
	}
	cmd.Flags().StringVar(&apiBase, "api-base-url", "", "Override API base URL")
	cmd.Flags().StringVar(&workspace, "workspace", "", "Preselect a workspace key")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "Do not try to open the browser automatically")
	return cmd
}

func NewLogoutCommand() *cobra.Command {
	var apiBase string

	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Revoke the refresh token and clear local credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			session, err := auth.ReadSession()
			if err != nil {
				return err
			}
			if session == nil {
				return emit(map[string]any{
					"authenticated": false,
					"cleared":       false,
					"session_file":  auth.SessionFilePath(),
				})
			}

			if hasInteractiveTTY() {
				userLabel := "current user"
				if session.User != nil && session.User.Email != "" {
					userLabel = session.User.Email
				}
				confirm := false
				if err := survey.AskOne(&survey.Confirm{
					Message: fmt.Sprintf("Sign out %s?", userLabel),
					Default: true,
				}, &confirm); err != nil {
					return err
				}
				if !confirm {
					return errors.New("cancelled")
				}
			}

			client := auth.NewClient(sessionAPIBase(apiBase, session))
			_ = client.Logout(session.RefreshToken)
			if err := client.ClearLocalSession(); err != nil {
				return err
			}

			return emit(map[string]any{
				"authenticated": false,
				"cleared":       true,
				"session_file":  auth.SessionFilePath(),
			})
		},
	}
	cmd.Flags().StringVar(&apiBase, "api-base-url", "", "Override API base URL")
	return cmd
}

func NewWhoAmICommand() *cobra.Command {
	var apiBase string

	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Show the current authenticated user and workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			session, err := auth.ReadSession()
			if err != nil {
				return err
			}
			if session == nil {
				return errors.New("you are not logged in")
			}
			client := auth.NewClient(sessionAPIBase(apiBase, session))
			session, err = client.WhoAmI()
			if err != nil {
				return err
			}
			report := BuildIdentityReport(session, &client.URLs)
			return emit(report)
		},
	}
	cmd.Flags().StringVar(&apiBase, "api-base-url", "", "Override API base URL")
	return cmd
}
