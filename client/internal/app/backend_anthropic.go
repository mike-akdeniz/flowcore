package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// AnthropicBackend asks Claude. It is offered only when a key is set.
//
// This file and the local backend are the only places in the application that
// know a model exists. FlowCore never calls one, and structurally cannot: it
// holds an opaque subject reference and not the subject, so it could not build
// the question without storing cases itself or calling back into this
// application. Both would invert the relationship between a library and its
// caller. What crosses back into the library is the same thing a person's click
// produces — an action id, a completer, and a remark.
type AnthropicBackend struct {
	client anthropic.Client
}

// NewAnthropicBackend takes the SDK's request options, so a test can point it at
// a fake server. Without options it reads ANTHROPIC_API_KEY, as the SDK does.
func NewAnthropicBackend(options ...option.RequestOption) *AnthropicBackend {
	return &AnthropicBackend{client: anthropic.NewClient(options...)}
}

func (b *AnthropicBackend) Name() string  { return "anthropic" }
func (b *AnthropicBackend) Label() string { return "Anthropic" }

// anthropicMaxTokens leaves room for thinking, which the newest models do
// whether asked to or not. Nothing else about the request depends on the model:
// no thinking or effort settings are sent, because models disagree about which
// they accept, and any listed model must accept the request (client decision 40).
const anthropicMaxTokens = 16000

// Models lists the models this key can use that support structured outputs —
// the one capability every question depends on.
func (b *AnthropicBackend) Models(ctx context.Context) ([]Model, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var models []Model

	pages := b.client.Models.ListAutoPaging(ctx, anthropic.ModelListParams{})
	for pages.Next() {
		model := pages.Current()
		if !model.Capabilities.StructuredOutputs.Supported {
			continue
		}

		models = append(models, Model{ID: model.ID, Label: model.DisplayName})
	}

	if err := pages.Err(); err != nil {
		return nil, fmt.Errorf("listing Anthropic models: %w", err)
	}

	return models, nil
}

func (b *AnthropicBackend) Decide(ctx context.Context, model string, question Question) (Answer, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	response, err := b.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: anthropicMaxTokens,
		System:    []anthropic.TextBlockParam{{Text: question.System}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(question.User)),
		},
		OutputConfig: anthropic.OutputConfigParam{
			Format: anthropic.JSONOutputFormatParam{Schema: question.Schema},
		},
	})
	if err != nil {
		return Answer{}, classifyAnthropic(model, err)
	}

	// Both are billed and both would happen again: the same case gets the same
	// refusal, and a reply that ran out of room will run out of room again.
	switch response.StopReason {
	case anthropic.StopReasonRefusal:
		return Answer{}, permanent(fmt.Errorf("%s declined to decide this step", model))
	case anthropic.StopReasonMaxTokens:
		return Answer{}, permanent(fmt.Errorf("%s's reply was cut off at %d tokens", model, anthropicMaxTokens))
	}

	var text strings.Builder
	for _, block := range response.Content {
		if paragraph, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(paragraph.Text)
		}
	}

	return parseAnswer(model, text.String())
}

// classifyAnthropic sorts a failed request. The SDK has already retried rate
// limits and server errors twice; one still failing is worth another try on the
// next sweep. Any other status is a request the API will reject again.
func classifyAnthropic(model string, err error) error {
	failure := fmt.Errorf("asking %s: %w", model, err)

	var apiError *anthropic.Error
	if !errors.As(err, &apiError) {
		// No status at all: the connection failed, or the deadline passed.
		return transient(failure)
	}

	if apiError.StatusCode == http.StatusTooManyRequests || apiError.StatusCode >= 500 {
		return transient(failure)
	}

	return permanent(failure)
}
