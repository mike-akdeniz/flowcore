package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mike-akdeniz/flowcore/client/internal/app"
)

// readModels asks for the picker's contents as a new visitor, and returns the
// listing and the session it seeded.
func readModels(t *testing.T, server *Server) (modelsJSON, string) {
	t.Helper()

	response := httptest.NewRecorder()
	server.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/models", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}

	var sessionID string
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == sessionCookie {
			sessionID = cookie.Value
			removeSession(t, server, sessionID)
		}
	}

	var models modelsJSON
	if err := json.Unmarshal(response.Body.Bytes(), &models); err != nil {
		t.Fatal(err)
	}

	return models, sessionID
}

// Run locally, the picker offers Replay first and starts empty: CaseWork does
// not choose for the visitor, even when Replay is all there is.
func TestPickerStartsEmptyLocally(t *testing.T) {
	server := configuredServer(t, testDatabase(t), app.Config{})
	models, _ := readModels(t, server)

	if len(models.Groups) == 0 || models.Groups[0].Backend != "replay" ||
		len(models.Groups[0].Models) != 1 || models.Groups[0].Models[0].Label != "Replay" {
		t.Errorf("groups %+v, want Replay first", models.Groups)
	}

	if models.Chosen != nil || models.Locked || models.Replaying {
		t.Errorf("chosen %+v, locked %v, want nothing chosen and the picker open", models.Chosen, models.Locked)
	}
}

// Hosted, the demo replays only: Replay is chosen, the picker is locked, and a
// key on the host would not add Anthropic's models.
func TestReplayOnlyLocksThePicker(t *testing.T) {
	server := configuredServer(t, testDatabase(t), app.Config{ReplayOnly: true, AnthropicAPIKey: "unused"})
	models, sessionID := readModels(t, server)

	if len(models.Groups) != 1 || models.Groups[0].Backend != "replay" {
		t.Errorf("groups %+v, want Replay alone", models.Groups)
	}

	if models.Chosen == nil || models.Chosen.Label != "Replay" || !models.Available || !models.Locked ||
		!models.Replaying {
		t.Errorf("chosen %+v, available %v, locked %v, want Replay chosen and locked",
			models.Chosen, models.Available, models.Locked)
	}

	refused := jsonRequest(t, server, sessionID, http.MethodPut, "/api/models/chosen",
		map[string]string{"value": "anthropic/claude-sonnet-5-5"}, http.StatusConflict)
	if !strings.Contains(refused.Body.String(), "not available") {
		t.Errorf("refusal %q", refused.Body.String())
	}
}
