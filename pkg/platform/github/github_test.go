package github

import (
	"testing"

	agshctx "github.com/cgast/agsh/pkg/context"
)

func TestExtractRepo(t *testing.T) {
	tests := []struct {
		name      string
		payload   any
		wantOwner string
		wantName  string
		wantErr   bool
	}{
		{
			name:      "string payload",
			payload:   "cgast/agsh",
			wantOwner: "cgast",
			wantName:  "agsh",
		},
		{
			name:      "map with repo key",
			payload:   map[string]any{"repo": "golang/go"},
			wantOwner: "golang",
			wantName:  "go",
		},
		{
			name:      "map with owner and name",
			payload:   map[string]any{"owner": "golang", "name": "go"},
			wantOwner: "golang",
			wantName:  "go",
		},
		{
			name:    "empty string",
			payload: "",
			wantErr: true,
		},
		{
			name:    "invalid format no slash",
			payload: "just-a-name",
			wantErr: true,
		},
		{
			name:    "nil payload",
			payload: nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := agshctx.NewEnvelope(tt.payload, "text/plain", "test")
			owner, name, err := extractRepo(env)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got owner=%q name=%q", owner, name)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if owner != tt.wantOwner {
				t.Errorf("owner = %q, want %q", owner, tt.wantOwner)
			}
			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}
		})
	}
}

func TestCommandIdentity(t *testing.T) {
	// Test that commands return correct identity info without needing a real client.
	// We use nil client since we're only testing metadata methods.
	repoInfo := &RepoInfoCommand{}
	if repoInfo.Name() != "github:repo:info" {
		t.Errorf("RepoInfoCommand.Name() = %q", repoInfo.Name())
	}
	if repoInfo.Namespace() != "github" {
		t.Errorf("RepoInfoCommand.Namespace() = %q", repoInfo.Namespace())
	}
	if len(repoInfo.RequiredCredentials()) != 1 || repoInfo.RequiredCredentials()[0] != "GITHUB_TOKEN" {
		t.Errorf("RepoInfoCommand.RequiredCredentials() = %v", repoInfo.RequiredCredentials())
	}

	prList := &PRListCommand{}
	if prList.Name() != "github:pr:list" {
		t.Errorf("PRListCommand.Name() = %q", prList.Name())
	}

	issueCreate := &IssueCreateCommand{}
	if issueCreate.Name() != "github:issue:create" {
		t.Errorf("IssueCreateCommand.Name() = %q", issueCreate.Name())
	}
}
