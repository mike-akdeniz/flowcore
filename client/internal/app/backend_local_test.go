package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// localServer stands in for llama-server: it records the last request and
// answers with whatever the test hands it.
func localServer(t *testing.T, status int, reply string) (*LocalBackend, *map[string]any) {
	t.Helper()

	var received map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gemma-3-270m"},{"id":"smollm2-360m"}]}`))
		case "/v1/chat/completions":
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

	return NewLocalBackend(server.URL + "/"), &received
}

func completion(finishReason, content string) string {
	body, _ := json.Marshal(map[string]any{
		"choices": []any{map[string]any{
			"finish_reason": finishReason,
			"message":       map[string]any{"role": "assistant", "content": content},
		}},
	})

	return string(body)
}

func testQuestion(t *testing.T) Question {
	t.Helper()

	question, err := NewQuestion(CheckRequest{
		StepName:     "estimate check",
		Instructions: stepInstructions("Decide whether the estimate can be assessed."),
		SubjectText:  "Claim: C-1042",
		Actions:      testActions("adequate", "needs detail"),
	})
	if err != nil {
		t.Fatal(err)
	}

	return question
}

func TestLocalBackendListsTheServersModels(t *testing.T) {
	backend, _ := localServer(t, http.StatusOK, "")

	models, err := backend.Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(models) != 2 || models[0].ID != "gemma-3-270m" || models[0].Label != "gemma-3-270m" {
		t.Errorf("models %+v", models)
	}
}

func TestLocalBackendSendsTheSchemaAndReadsTheReply(t *testing.T) {
	backend, received := localServer(t, http.StatusOK,
		completion("stop", `{"action":"needs detail","finding":"One figure, no breakdown."}`))

	answer, err := backend.Decide(context.Background(), "gemma-3-270m", testQuestion(t))
	if err != nil {
		t.Fatal(err)
	}

	if answer != (Answer{Action: "needs detail", Finding: "One figure, no breakdown."}) {
		t.Errorf("answer %+v", answer)
	}

	request := *received
	if request["model"] != "gemma-3-270m" {
		t.Errorf("asked model %v", request["model"])
	}

	messages := request["messages"].([]any)
	if len(messages) != 2 || messages[0].(map[string]any)["role"] != "system" ||
		messages[1].(map[string]any)["content"] != "Claim: C-1042" {
		t.Errorf("messages %v", messages)
	}

	format := request["response_format"].(map[string]any)
	schema := format["json_schema"].(map[string]any)["schema"].(map[string]any)
	properties := schema["properties"].(map[string]any)
	enum := properties["action"].(map[string]any)["enum"].([]any)
	if format["type"] != "json_schema" || len(enum) != 2 {
		t.Errorf("response_format %v", format)
	}

	// Two things only a small local model needs: a finding it cannot stop after a
	// few words, and no sampling, so the same case reads the same way twice.
	if properties["finding"].(map[string]any)["minLength"] != float64(localFindingMinimum) {
		t.Errorf("finding %v, want a minimum length", properties["finding"])
	}

	if request["temperature"] != float64(0) {
		t.Errorf("temperature %v, want 0", request["temperature"])
	}
}

func TestLocalBackendLeavesTheSharedSchemaAlone(t *testing.T) {
	question := testQuestion(t)
	backend, _ := localServer(t, http.StatusOK, completion("stop", `{"action":"adequate","finding":"x"}`))

	if _, err := backend.Decide(context.Background(), "gemma-3-270m", question); err != nil {
		t.Fatal(err)
	}

	finding := question.Schema["properties"].(map[string]any)["finding"].(map[string]any)
	if _, changed := finding["minLength"]; changed {
		t.Error("the local backend's minimum length leaked into the question every backend shares")
	}
}

func TestLocalBackendSortsFailures(t *testing.T) {
	for name, test := range map[string]struct {
		status    int
		reply     string
		permanent bool
	}{
		"server error":        {http.StatusInternalServerError, `{"error":"boom"}`, false},
		"busy":                {http.StatusServiceUnavailable, `{"error":"loading model"}`, false},
		"rejected request":    {http.StatusBadRequest, `{"error":"bad schema"}`, true},
		"cut off":             {http.StatusOK, completion("length", `{"action":"adeq`), true},
		"schema not enforced": {http.StatusOK, completion("stop", `I think it is adequate.`), true},
		"no action":           {http.StatusOK, completion("stop", `{"action":"","finding":"x"}`), true},
		"unreadable envelope": {http.StatusOK, `not json`, false},
		"no choices":          {http.StatusOK, `{"choices":[]}`, true},
	} {
		backend, _ := localServer(t, test.status, test.reply)

		_, err := backend.Decide(context.Background(), "gemma-3-270m", testQuestion(t))
		if err == nil {
			t.Errorf("%s: no error", name)

			continue
		}

		if isPermanent(err) != test.permanent {
			t.Errorf("%s: permanent = %v, want %v (%v)", name, isPermanent(err), test.permanent, err)
		}
	}
}

func TestLocalBackendUnreachableIsTransient(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	backend := NewLocalBackend(server.URL)
	server.Close()

	if _, err := backend.Models(context.Background()); err == nil {
		t.Error("listing a stopped server succeeded")
	}

	_, err := backend.Decide(context.Background(), "gemma-3-270m", testQuestion(t))
	if err == nil || isPermanent(err) {
		t.Errorf("err = %v, want a transient failure", err)
	}
}
