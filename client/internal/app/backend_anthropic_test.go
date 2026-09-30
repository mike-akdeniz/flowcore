package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
)

// anthropicServer stands in for the Messages and Models APIs, so the backend is
// tested without a key and without spending anything: nothing in this
// application's verification calls Anthropic (client decision 40).
func anthropicServer(t *testing.T, status int, reply string) (*AnthropicBackend, *map[string]any) {
	t.Helper()

	var received map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[
				{"type":"model","id":"claude-opus-5-5","display_name":"Claude Opus 5.5",
				 "created_at":"2026-08-01T00:00:00Z","capabilities":{"structured_outputs":{"supported":true}}},
				{"type":"model","id":"claude-old","display_name":"Claude Old",
				 "created_at":"2024-01-01T00:00:00Z","capabilities":{"structured_outputs":{"supported":false}}}
			],"has_more":false,"first_id":"claude-opus-5-5","last_id":"claude-old"}`))
		case "/v1/messages":
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Error(err)
			}

			w.WriteHeader(status)
			_, _ = w.Write([]byte(reply))
		default:
			http.NotFound(w, r)
		}
	}))

	t.Cleanup(server.Close)

	// No retries, so a failure is reported at once rather than after the SDK's
	// backoff; the classification is what is under test.
	backend := NewAnthropicBackend(
		option.WithBaseURL(server.URL),
		option.WithAPIKey("test-key"),
		option.WithMaxRetries(0))

	return backend, &received
}

func message(stopReason, text string) string {
	body, _ := json.Marshal(map[string]any{
		"id": "msg_test", "type": "message", "role": "assistant", "model": "claude-opus-5-5",
		"content":       []any{map[string]any{"type": "text", "text": text}},
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"usage":         map[string]any{"input_tokens": 10, "output_tokens": 10},
	})

	return string(body)
}

func apiError(kind string) string {
	return `{"type":"error","error":{"type":"` + kind + `","message":"test"}}`
}

func TestAnthropicBackendListsModelsThatCanFollowASchema(t *testing.T) {
	backend, _ := anthropicServer(t, http.StatusOK, "")

	models, err := backend.Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(models) != 1 || models[0].ID != "claude-opus-5-5" || models[0].Label != "Claude Opus 5.5" {
		t.Errorf("models %+v, want only the one supporting structured outputs", models)
	}
}

func TestAnthropicBackendSendsTheSchemaAndReadsTheReply(t *testing.T) {
	backend, received := anthropicServer(t, http.StatusOK,
		message("end_turn", `{"action":"adequate","finding":"Itemised throughout."}`))

	answer, err := backend.Decide(context.Background(), "claude-opus-5-5", testQuestion(t))
	if err != nil {
		t.Fatal(err)
	}

	if answer != (Answer{Action: "adequate", Finding: "Itemised throughout."}) {
		t.Errorf("answer %+v", answer)
	}

	request := *received
	if request["model"] != "claude-opus-5-5" {
		t.Errorf("asked model %v", request["model"])
	}

	format := request["output_config"].(map[string]any)["format"].(map[string]any)
	if format["type"] != "json_schema" || format["schema"] == nil {
		t.Errorf("output_config.format %v", format)
	}

	// Whatever the model, the request carries nothing a model might reject.
	for _, field := range []string{"thinking", "temperature"} {
		if _, sent := request[field]; sent {
			t.Errorf("sent %s", field)
		}
	}

	if _, sent := request["output_config"].(map[string]any)["effort"]; sent {
		t.Error("sent an effort level")
	}
}

func TestAnthropicBackendSortsFailures(t *testing.T) {
	for name, test := range map[string]struct {
		status    int
		reply     string
		permanent bool
	}{
		"rate limited":     {http.StatusTooManyRequests, apiError("rate_limit_error"), false},
		"overloaded":       {529, apiError("overloaded_error"), false},
		"server error":     {http.StatusInternalServerError, apiError("api_error"), false},
		"rejected request": {http.StatusBadRequest, apiError("invalid_request_error"), true},
		"bad key":          {http.StatusUnauthorized, apiError("authentication_error"), true},
		"refused":          {http.StatusOK, message("refusal", ""), true},
		"cut off":          {http.StatusOK, message("max_tokens", `{"action":"adeq`), true},
	} {
		backend, _ := anthropicServer(t, test.status, test.reply)

		_, err := backend.Decide(context.Background(), "claude-opus-5-5", testQuestion(t))
		if err == nil {
			t.Errorf("%s: no error", name)

			continue
		}

		if isPermanent(err) != test.permanent {
			t.Errorf("%s: permanent = %v, want %v (%v)", name, isPermanent(err), test.permanent, err)
		}
	}
}
