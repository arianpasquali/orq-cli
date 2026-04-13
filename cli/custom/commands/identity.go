package commands

import (
	"orq/cli/custom/auth"
)

type IdentityWorkspace struct {
	ID           string `json:"id"`
	Key          string `json:"key"`
	Name         string `json:"name"`
	TotalMembers int    `json:"total_members"`
}

type IdentityUser struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

type IdentityReport struct {
	Authenticated      bool                `json:"authenticated"`
	SessionFile        string              `json:"session_file"`
	User               *IdentityUser       `json:"user"`
	ActiveWorkspaceKey *string             `json:"active_workspace_key"`
	Workspaces         []IdentityWorkspace `json:"workspaces"`
	URLs               *auth.URLs          `json:"urls,omitempty"`
}

func BuildIdentityReport(session *auth.Session, urls *auth.URLs) IdentityReport {
	rep := IdentityReport{
		Authenticated:      true,
		SessionFile:        auth.SessionFilePath(),
		ActiveWorkspaceKey: session.ActiveWorkspaceKey,
		URLs:               urls,
		Workspaces:         []IdentityWorkspace{},
	}
	if session.User != nil {
		rep.User = &IdentityUser{
			ID:          session.User.ID,
			Email:       session.User.Email,
			DisplayName: session.User.DisplayName,
		}
	}
	for _, w := range session.Workspaces {
		rep.Workspaces = append(rep.Workspaces, workspaceFromMap(w))
	}
	return rep
}

func workspaceFromMap(w map[string]any) IdentityWorkspace {
	out := IdentityWorkspace{}
	if v, ok := w["id"].(string); ok {
		out.ID = v
	}
	if v, ok := w["key"].(string); ok {
		out.Key = v
	}
	if v, ok := w["name"].(string); ok {
		out.Name = v
	}
	if v, ok := w["total_members"].(float64); ok {
		out.TotalMembers = int(v)
	}
	return out
}
