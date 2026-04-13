package commands

import (
	"errors"
	"fmt"
	"os"

	"orq/cli/custom/auth"

	survey "github.com/AlecAivazis/survey/v2"
	isatty "github.com/mattn/go-isatty"
)

func hasInteractiveTTY() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd())
}

func selectWorkspace(workspaces []map[string]any, message string) (string, error) {
	if len(workspaces) == 0 {
		return "", errors.New("no workspaces available")
	}
	if len(workspaces) == 1 {
		if k, ok := workspaces[0]["key"].(string); ok {
			return k, nil
		}
		return "", errors.New("workspace is missing a key")
	}
	options := make([]string, 0, len(workspaces))
	keys := make([]string, 0, len(workspaces))
	for _, w := range workspaces {
		name, _ := w["name"].(string)
		key, _ := w["key"].(string)
		options = append(options, fmt.Sprintf("%s (%s)", name, key))
		keys = append(keys, key)
	}
	var chosen string
	if err := survey.AskOne(&survey.Select{
		Message: message,
		Options: options,
	}, &chosen); err != nil {
		return "", err
	}
	for i, opt := range options {
		if opt == chosen {
			return keys[i], nil
		}
	}
	return "", errors.New("no workspace selected")
}

func sessionAPIBase(override string, session *auth.Session) string {
	if override != "" {
		return override
	}
	if session != nil {
		return session.APIBaseURL
	}
	return ""
}
