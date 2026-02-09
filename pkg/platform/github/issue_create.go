package github

import (
	gocontext "context"
	"fmt"

	gh "github.com/google/go-github/v60/github"

	agshctx "github.com/cgast/agsh/pkg/context"
	"github.com/cgast/agsh/pkg/platform"
)

// IssueCreateCommand implements github:issue:create — creates a new issue.
type IssueCreateCommand struct {
	client *Client
}

// NewIssueCreateCommand creates a new github:issue:create command.
func NewIssueCreateCommand(client *Client) *IssueCreateCommand {
	return &IssueCreateCommand{client: client}
}

func (c *IssueCreateCommand) Name() string        { return "github:issue:create" }
func (c *IssueCreateCommand) Description() string { return "Create a new issue in a repository" }
func (c *IssueCreateCommand) Namespace() string   { return "github" }

func (c *IssueCreateCommand) InputSchema() platform.Schema {
	return platform.Schema{
		Type: "object",
		Properties: map[string]platform.SchemaField{
			"repo":   {Type: "string", Description: "Repository in owner/name format"},
			"title":  {Type: "string", Description: "Issue title"},
			"body":   {Type: "string", Description: "Issue body (markdown)"},
			"labels": {Type: "array", Description: "Labels to apply"},
		},
		Required: []string{"repo", "title"},
	}
}

func (c *IssueCreateCommand) OutputSchema() platform.Schema {
	return platform.Schema{
		Type: "object",
		Properties: map[string]platform.SchemaField{
			"number":   {Type: "integer", Description: "Issue number"},
			"html_url": {Type: "string", Description: "URL of the created issue"},
		},
	}
}

func (c *IssueCreateCommand) RequiredCredentials() []string {
	return []string{"GITHUB_TOKEN"}
}

func (c *IssueCreateCommand) Execute(ctx gocontext.Context, input agshctx.Envelope, _ agshctx.ContextStore) (agshctx.Envelope, error) {
	owner, name, err := extractRepo(input)
	if err != nil {
		return agshctx.Envelope{}, fmt.Errorf("github:issue:create: %w", err)
	}

	m, ok := input.Payload.(map[string]any)
	if !ok {
		return agshctx.Envelope{}, fmt.Errorf("github:issue:create: expected map payload with 'title'")
	}

	title, _ := m["title"].(string)
	if title == "" {
		return agshctx.Envelope{}, fmt.Errorf("github:issue:create: missing 'title'")
	}

	body, _ := m["body"].(string)

	issueReq := &gh.IssueRequest{
		Title: &title,
	}
	if body != "" {
		issueReq.Body = &body
	}

	// Handle labels if provided.
	if labelsRaw, ok := m["labels"]; ok {
		if labels, ok := labelsRaw.([]any); ok {
			labelStrs := make([]string, 0, len(labels))
			for _, l := range labels {
				if s, ok := l.(string); ok {
					labelStrs = append(labelStrs, s)
				}
			}
			if len(labelStrs) > 0 {
				issueReq.Labels = &labelStrs
			}
		}
	}

	issue, _, err := c.client.inner.Issues.Create(ctx, owner, name, issueReq)
	if err != nil {
		return agshctx.Envelope{}, fmt.Errorf("github:issue:create: API error: %w", err)
	}

	result := map[string]any{
		"number":     issue.GetNumber(),
		"title":      issue.GetTitle(),
		"html_url":   issue.GetHTMLURL(),
		"state":      issue.GetState(),
		"created_at": issue.GetCreatedAt().String(),
	}

	env := agshctx.NewEnvelope(result, "application/json", "github:issue:create")
	env.Meta.Tags["repo"] = owner + "/" + name
	env.Meta.Tags["issue_number"] = fmt.Sprintf("%d", issue.GetNumber())
	return env, nil
}
