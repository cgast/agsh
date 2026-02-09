package github

import (
	gocontext "context"
	"fmt"
	"strings"

	agshctx "github.com/cgast/agsh/pkg/context"
	"github.com/cgast/agsh/pkg/platform"
)

// RepoInfoCommand implements github:repo:info — fetches repository information.
type RepoInfoCommand struct {
	client *Client
}

// NewRepoInfoCommand creates a new github:repo:info command.
func NewRepoInfoCommand(client *Client) *RepoInfoCommand {
	return &RepoInfoCommand{client: client}
}

func (c *RepoInfoCommand) Name() string        { return "github:repo:info" }
func (c *RepoInfoCommand) Description() string { return "Get repository information" }
func (c *RepoInfoCommand) Namespace() string   { return "github" }

func (c *RepoInfoCommand) InputSchema() platform.Schema {
	return platform.Schema{
		Type: "object",
		Properties: map[string]platform.SchemaField{
			"repo": {Type: "string", Description: "Repository in owner/name format"},
		},
		Required: []string{"repo"},
	}
}

func (c *RepoInfoCommand) OutputSchema() platform.Schema {
	return platform.Schema{
		Type: "object",
		Properties: map[string]platform.SchemaField{
			"name":         {Type: "string", Description: "Repository name"},
			"full_name":    {Type: "string", Description: "Full repository name (owner/name)"},
			"description":  {Type: "string", Description: "Repository description"},
			"stars":        {Type: "integer", Description: "Star count"},
			"forks":        {Type: "integer", Description: "Fork count"},
			"open_issues":  {Type: "integer", Description: "Open issue count"},
			"language":     {Type: "string", Description: "Primary language"},
			"default_branch": {Type: "string", Description: "Default branch name"},
		},
	}
}

func (c *RepoInfoCommand) RequiredCredentials() []string {
	return []string{"GITHUB_TOKEN"}
}

func (c *RepoInfoCommand) Execute(ctx gocontext.Context, input agshctx.Envelope, _ agshctx.ContextStore) (agshctx.Envelope, error) {
	owner, name, err := extractRepo(input)
	if err != nil {
		return agshctx.Envelope{}, fmt.Errorf("github:repo:info: %w", err)
	}

	repo, _, err := c.client.inner.Repositories.Get(ctx, owner, name)
	if err != nil {
		return agshctx.Envelope{}, fmt.Errorf("github:repo:info: API error: %w", err)
	}

	result := map[string]any{
		"name":           repo.GetName(),
		"full_name":      repo.GetFullName(),
		"description":    repo.GetDescription(),
		"stars":          repo.GetStargazersCount(),
		"forks":          repo.GetForksCount(),
		"open_issues":    repo.GetOpenIssuesCount(),
		"language":       repo.GetLanguage(),
		"default_branch": repo.GetDefaultBranch(),
		"html_url":       repo.GetHTMLURL(),
		"created_at":     repo.GetCreatedAt().Time.String(),
		"updated_at":     repo.GetUpdatedAt().Time.String(),
	}

	env := agshctx.NewEnvelope(result, "application/json", "github:repo:info")
	env.Meta.Tags["repo"] = owner + "/" + name
	return env, nil
}

// extractRepo gets owner/name from the input envelope.
func extractRepo(input agshctx.Envelope) (string, string, error) {
	var repoStr string

	switch v := input.Payload.(type) {
	case string:
		repoStr = v
	case map[string]any:
		if r, ok := v["repo"]; ok {
			if s, ok := r.(string); ok {
				repoStr = s
			}
		}
		// Also support separate owner/name fields.
		if repoStr == "" {
			owner, _ := v["owner"].(string)
			name, _ := v["name"].(string)
			if owner != "" && name != "" {
				return owner, name, nil
			}
		}
	}

	if repoStr == "" {
		return "", "", fmt.Errorf("missing repo (expected 'owner/name' format)")
	}

	parts := strings.SplitN(repoStr, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repo format %q (expected 'owner/name')", repoStr)
	}
	return parts[0], parts[1], nil
}
