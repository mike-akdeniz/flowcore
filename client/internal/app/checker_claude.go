package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/mike-akdeniz/flowcore"
)

// ClaudeChecker asks a model to decide an agent step.
//
// This file is the only place in the application that knows a model exists.
// FlowCore never calls one, and structurally cannot: it holds an opaque subject
// reference and not the subject, so it could not build this prompt without either
// storing releases itself or calling back into this application. Both would
// invert the relationship between a library and its caller.
//
// What crosses back into the library is the same thing a human's click produces —
// an action id, a completer, and a remark.
type ClaudeChecker struct {
	client anthropic.Client
	model  string
}

func NewClaudeChecker() *ClaudeChecker {
	return &ClaudeChecker{client: anthropic.NewClient(), model: "claude-opus-5"}
}

func (c *ClaudeChecker) Mode() string { return "claude (" + c.model + ")" }

func (c *ClaudeChecker) Check(ctx context.Context, request CheckRequest) (Verdict, error) {
	// From the step, frozen at start. The agent reference only says which agent
	// holds the step; what it is asked to do is configuration, and it lives in
	// the workflow where an editor can see and change it.
	if request.Instructions == nil {
		return Verdict{}, fmt.Errorf("%q has no instructions for %s", request.StepName, request.Agent)
	}

	instruction := *request.Instructions

	names := make([]string, 0, len(request.Actions))
	for _, action := range request.Actions {
		names = append(names, action.Name)
	}

	// The available actions come from FlowCore, so the model is choosing from the
	// workflow as it was defined — not from a list hard-coded here that could
	// drift away from the definition.
	system := instruction + "\n\nReply in exactly this form and nothing else:\n\n" +
		"ACTION: <one of: " + strings.Join(names, ", ") + ">\n" +
		"FINDING: <two or three sentences: what you found, or that you found nothing>"

	response, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     c.model,
		MaxTokens: 1024,
		System:    []anthropic.TextBlockParam{{Text: system}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(request.SubjectText)),
		},
	})
	if err != nil {
		return Verdict{}, fmt.Errorf("asking %s: %w", c.model, err)
	}

	var text strings.Builder
	for _, block := range response.Content {
		if paragraph, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(paragraph.Text)
		}
	}

	return parseVerdict(text.String(), request.Actions)
}

// parseVerdict reads the model's answer and checks it against what the step
// actually offers.
//
// The validation is not defensive clutter: a model asked to pick from a list will
// occasionally return something adjacent, and the alternative to catching it here
// is a foreign-key violation from the database with nothing explaining why.
func parseVerdict(text string, actions []flowcore.Action) (Verdict, error) {
	var chosen, finding string

	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, "ACTION:"):
			chosen = strings.TrimSpace(strings.TrimPrefix(line, "ACTION:"))
		case strings.HasPrefix(line, "FINDING:"):
			finding = strings.TrimSpace(strings.TrimPrefix(line, "FINDING:"))
		}
	}

	if chosen == "" {
		return Verdict{}, fmt.Errorf("no ACTION line in the reply: %q", truncate(text, 200))
	}

	actionID, err := actionNamed(actions, chosen)
	if err != nil {
		return Verdict{}, err
	}

	return Verdict{ActionID: actionID, Remark: finding}, nil
}

func truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}

	return text[:limit] + "…"
}
