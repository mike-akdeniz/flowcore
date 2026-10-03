package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// A person's step carries its instructions to the case screen; an agent step's
// are its prompt, and do not.
func TestOnlyAPersonsStepShowsItsInstructions(t *testing.T) {
	server, sessionID := documentTestServer(t)

	caseRequest(t, server, sessionID, http.MethodPost, "/api/cases/C-1042/submit", http.StatusOK)

	step := readCase(t, server, sessionID).CurrentStep
	if step == nil || !step.IsAgent {
		t.Fatalf("the submitted claim is at %+v, want its agent triage step", step)
	}

	if step.Instructions != nil {
		t.Errorf("the agent step sent its prompt to the case screen: %q", *step.Instructions)
	}

	// Handed to a person, the same step is theirs to decide, and they are shown
	// what the agent would have been asked.
	response := postJSON(t, server, sessionID, "/api/cases/C-1042/reassign",
		encode(t, map[string]string{"visitId": step.VisitID, "assignee": "group:claims-adjusters"}),
		http.StatusOK)

	var subject caseJSON
	if err := json.Unmarshal(response.Body.Bytes(), &subject); err != nil {
		t.Fatal(err)
	}

	reassigned := subject.CurrentStep
	if reassigned == nil || reassigned.Instructions == nil ||
		!strings.HasPrefix(*reassigned.Instructions, "Decide whether this claim can take the fast track") {
		t.Errorf("the reassigned step is %+v, want triage's instructions", reassigned)
	}
}
