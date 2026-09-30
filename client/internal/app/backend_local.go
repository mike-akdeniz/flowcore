package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// LocalBackend is a model server on this machine speaking the OpenAI-compatible
// API — llama.cpp's `llama-server` as the README sets it up, or Ollama, which
// answers the same two endpoints.
//
// Plain net/http, not an SDK: two requests, and no dependency that knows about
// either server (client decision 40).
type LocalBackend struct {
	baseURL string
	client  *http.Client
}

func NewLocalBackend(baseURL string) *LocalBackend {
	return &LocalBackend{
		baseURL: strings.TrimRight(baseURL, "/"),
		// Generous, because a small model on a laptop CPU is still a model on a
		// laptop CPU, and a call cut off here is retried from the start.
		client: &http.Client{Timeout: 2 * time.Minute},
	}
}

func (b *LocalBackend) Name() string  { return "local" }
func (b *LocalBackend) Label() string { return "Local" }

// localMaxTokens bounds the reply. The finding is asked to be two or three
// sentences; the bound only stops a model that ignores that from running on.
const localMaxTokens = 1024

// localTemperature is set here because llama-server samples at 0.8 unless told
// otherwise, and at that a model this small writes a different finding, of very
// different quality, on every run of the same case.
const localTemperature = 0

func (b *LocalBackend) Models(ctx context.Context) ([]Model, error) {
	// Short, whatever the caller's deadline: this is asked while a page loads,
	// and a server that is not running should cost the page nothing.
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, b.baseURL+"/v1/models", nil)
	if err != nil {
		return nil, err
	}

	response, err := b.client.Do(request)
	if err != nil {
		return nil, err
	}

	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("listing local models: %s", response.Status)
	}

	var listing struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(response.Body).Decode(&listing); err != nil {
		return nil, fmt.Errorf("listing local models: %w", err)
	}

	models := make([]Model, 0, len(listing.Data))
	for _, model := range listing.Data {
		models = append(models, Model{ID: model.ID, Label: model.ID})
	}

	return models, nil
}

type localMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type localRequest struct {
	Model          string              `json:"model"`
	Messages       []localMessage      `json:"messages"`
	MaxTokens      int                 `json:"max_tokens"`
	Temperature    float64             `json:"temperature"`
	ResponseFormat localResponseFormat `json:"response_format"`
}

type localResponseFormat struct {
	Type       string          `json:"type"`
	JSONSchema localJSONSchema `json:"json_schema"`
}

type localJSONSchema struct {
	Name   string         `json:"name"`
	Strict bool           `json:"strict"`
	Schema map[string]any `json:"schema"`
}

func (b *LocalBackend) Decide(ctx context.Context, model string, question Question) (Answer, error) {
	body, err := json.Marshal(localRequest{
		Model: model,
		Messages: []localMessage{
			{Role: "system", Content: question.System},
			{Role: "user", Content: question.User},
		},
		MaxTokens:   localMaxTokens,
		Temperature: localTemperature,
		ResponseFormat: localResponseFormat{
			Type:       "json_schema",
			JSONSchema: localJSONSchema{Name: "verdict", Strict: true, Schema: withLongerFinding(question.Schema)},
		},
	})
	if err != nil {
		return Answer{}, permanent(err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, b.baseURL+"/v1/chat/completions",
		bytes.NewReader(body))
	if err != nil {
		return Answer{}, permanent(err)
	}

	request.Header.Set("Content-Type", "application/json")

	response, err := b.client.Do(request)
	if err != nil {
		return Answer{}, transient(fmt.Errorf("asking %s: %w", model, err))
	}

	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 500))
		failure := fmt.Errorf("asking %s: %s: %s", model, response.Status, strings.TrimSpace(string(detail)))

		// A busy or failing server may answer differently next time; a request it
		// rejected will be rejected again.
		if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 {
			return Answer{}, transient(failure)
		}

		return Answer{}, permanent(failure)
	}

	var completion struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(response.Body).Decode(&completion); err != nil {
		return Answer{}, transient(fmt.Errorf("reading %s's reply: %w", model, err))
	}

	if len(completion.Choices) == 0 {
		return Answer{}, permanent(fmt.Errorf("%s returned no reply", model))
	}

	choice := completion.Choices[0]
	if choice.FinishReason == "length" {
		return Answer{}, permanent(fmt.Errorf("%s's reply was cut off at %d tokens", model, localMaxTokens))
	}

	return parseAnswer(model, choice.Message.Content)
}

// localFindingMinimum is the shortest finding a local model may write, in
// characters. Left to itself a model this small often stops after a few words —
// "P-2087", "Claimant's account" — and a finding is the only thing that says why
// the step went the way it did.
const localFindingMinimum = 80

// withLongerFinding copies the question's schema with a minimum length on the
// finding. Only here: the constraint is for small local models, and a hosted API
// need not accept every JSON Schema keyword a local grammar does.
func withLongerFinding(schema map[string]any) map[string]any {
	properties := make(map[string]any)
	for name, property := range schema["properties"].(map[string]any) {
		properties[name] = property
	}

	properties["finding"] = map[string]any{"type": "string", "minLength": localFindingMinimum}

	copied := make(map[string]any, len(schema))
	for key, value := range schema {
		copied[key] = value
	}

	copied["properties"] = properties

	return copied
}

// parseAnswer reads a reply the schema constrained. Failing to parse it means the
// server did not enforce the schema, which asking again will not change.
func parseAnswer(model, content string) (Answer, error) {
	var answer Answer
	if err := json.Unmarshal([]byte(content), &answer); err != nil {
		return Answer{}, permanent(fmt.Errorf("%s's reply did not match the schema: %q", model, truncate(content, 200)))
	}

	if answer.Action == "" {
		return Answer{}, permanent(errors.New(model + "'s reply chose no action"))
	}

	return answer, nil
}

func truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}

	return text[:limit] + "…"
}
