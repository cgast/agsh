package github

import (
	gocontext "context"
	"fmt"

	gh "github.com/google/go-github/v60/github"

	agshctx "github.com/cgast/agsh/pkg/context"
	"github.com/cgast/agsh/pkg/platform"
)

// PRListCommand implements github:pr:list — lists pull requests for a repository.
type PRListCommand struct {
	client *Client
}

// NewPRListCommand creates a new github:pr:list command.
func NewPRListCommand(client *Client) *PRListCommand {
	return &PRListCommand{client: client}
}

func (c *PRListCommand) Name() string        { return "github:pr:list" }
func (c *PRListCommand) Description() string { return "List pull requests for a repository" }
func (c *PRListCommand) Namespace() string   { return "github" }

func (c *PRListCommand) InputSchema() platform.Schema {
	return platform.Schema{
		Type: "object",
		Properties: map[string]platform.SchemaField{
			"repo":  {Type: "string", Description: "Repository in owner/name format"},
			"state": {Type: "string", Description: "Filter by state: open, closed, all (default: open)"},
		},
		Required: []string{"repo"},
	}
}

func (c *PRListCommand) OutputSchema() platform.Schema {
	return platform.Schema{
		Type: "object",
		Properties: map[string]platform.SchemaField{
			"pull_requests": {Type: "array", Description: "List of pull requests"},
			"count":         {Type: "integer", Description: "Number of pull requests"},
		},
	}
}

func (c *PRListCommand) RequiredCredentials() []string {
	return []string{"GITHUB_TOKEN"}
}

func (c *PRListCommand) Execute(ctx gocontext.Context, input agshctx.Envelope, _ agshctx.ContextStore) (agshctx.Envelope, error) {
	owner, name, err := extractRepo(input)
	if err != nil {
		return agshctx.Envelope{}, fmt.Errorf("github:pr:list: %w", err)
	}

	state := "open"
	if m, ok := input.Payload.(map[string]any); ok {
		if s, ok := m["state"].(string); ok && s != "" {
			state = s
		}
	}

	opts := &gh.PullRequestListOptions{
		State:       state,
		ListOptions: gh.ListOptions{PerPage: 100},
	}

	prs, _, err := c.client.inner.PullRequests.List(ctx, owner, name, opts)
	if err != nil {
		return agshctx.Envelope{}, fmt.Errorf("github:pr:list: API error: %w", err)
	}

	items := make([]map[string]any, 0, len(prs))
	for _, pr := range prs {
		items = append(items, map[string]any{
			"number":     pr.GetNumber(),
			"title":      pr.GetTitle(),
			"state":      pr.GetState(),
			"author":     pr.GetUser().GetLogin(),
			"created_at": pr.GetCreatedAt().String(),
			"updated_at": pr.GetUpdatedAt().String(),
			"html_url":   pr.GetHTMLURL(),
			"draft":      pr.GetDraft(),
		})
	}

	result := map[string]any{
		"pull_requests": items,
		"count":         len(items),
	}

	env := agshctx.NewEnvelope(result, "application/json", "github:pr:list")
	env.Meta.Tags["repo"] = owner + "/" + name
	env.Meta.Tags["state"] = state
	env.Meta.Tags["count"] = fmt.Sprintf("%d", len(items))
	return env, nil
}
