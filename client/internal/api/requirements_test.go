package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/mike-akdeniz/flowcore"
	"github.com/mike-akdeniz/flowcore/client/internal/app"
	"github.com/mike-akdeniz/flowcore/client/internal/store"
)

// Tests for "required means required" at run time (client decision 36): a
// decision waits for its step's documents, a handoff to an AI step waits for the
// AI step's, a case cannot start at an AI step that lacks them, and only whoever
// holds a submitted case may file on it. Requirements come from the run's
// snapshot, so the tests edit the definition mid-run to show it does not reach.

// requestAs is jsonRequest signed in as a member of the cast.
func requestAs(
	t *testing.T,
	server *Server,
	sessionID, member, method, path string,
	body any,
	status int,
) *httptest.ResponseRecorder {
	t.Helper()

	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: sessionID})
	request.AddCookie(&http.Cookie{Name: "casework_identity", Value: member})
	response := httptest.NewRecorder()
	server.Routes().ServeHTTP(response, request)
	if response.Code != status {
		t.Fatalf("%s %s as %s: status %d, want %d: %s",
			method, path, member, response.Code, status, response.Body.String())
	}

	return response
}

// runnableClaim is the seeded session with its cast, ready to submit C-1042.
func runnableClaim(t *testing.T) (*Server, string) {
	t.Helper()

	server, sessionID := documentTestServer(t)
	if err := server.app.Store.SeedStaff(context.Background()); err != nil {
		t.Fatal(err)
	}

	return server, sessionID
}

// completeAsAIStep decides the open AI step the way the dispatcher would,
// through the same gate, without starting a worker.
func completeAsAIStep(t *testing.T, server *Server, sessionID, action string) {
	t.Helper()

	ctx := context.Background()
	submission, err := server.app.Store.SubmissionByReference(ctx, sessionID, "C-1042")
	if err != nil {
		t.Fatal(err)
	}

	state, err := server.app.Engine.GetState(ctx, *submission.SubjectReference, *submission.FlowcoreDefinitionID)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := server.app.CompleteStep(ctx, sessionID, submission, *state.CurrentStep,
		app.Identity{Reference: state.CurrentStep.AssigneeID},
		app.CompleteRequest{
			VisitID:             state.CurrentStep.VisitID,
			ActionID:            actionCalled(t, state, action),
			SubjectVersionToken: strconv.Itoa(submission.Revision),
		}); err != nil {
		t.Fatalf("%s: %v", state.CurrentStep.Name, err)
	}
}

func decision(t *testing.T, subject caseJSON, action string) decideJSON {
	t.Helper()

	if subject.CurrentStep == nil {
		t.Fatal("the case has no open step")
	}

	for _, candidate := range subject.CurrentStep.Actions {
		if candidate.Name == action {
			return decideJSON{VisitID: subject.CurrentStep.VisitID, ActionID: candidate.ID}
		}
	}

	t.Fatalf("no action %q on %q", action, subject.CurrentStep.Name)

	return decideJSON{}
}

func TestSubmitRefusesAnAIStepEntryWithoutItsDocuments(t *testing.T) {
	server, sessionID := runnableClaim(t)
	ctx := context.Background()

	var intakeNote string
	for _, document := range readCase(t, server, sessionID).Documents {
		if document.Kind == "intake-note" {
			intakeNote = document.ID
		}
	}

	caseRequest(t, server, sessionID, http.MethodDelete, "/api/cases/C-1042/documents/"+intakeNote, http.StatusOK)

	refused := caseRequest(t, server, sessionID, http.MethodPost, "/api/cases/C-1042/submit", http.StatusBadRequest)
	if !strings.Contains(refused.Body.String(), "Intake note") {
		t.Errorf("refusal does not name the missing type: %s", refused.Body.String())
	}

	if status := readCase(t, server, sessionID).Status; status != "draft" {
		t.Errorf("case is %q after a refused submission, want draft", status)
	}

	submission, err := server.app.Store.SubmissionByReference(ctx, sessionID, "C-1042")
	if err != nil {
		t.Fatal(err)
	}

	_, err = server.app.Engine.GetState(ctx, app.SubjectReference(sessionID, submission),
		activeDefinition(t, server, sessionID))
	if !errors.Is(err, flowcore.ErrNotFound) {
		t.Errorf("a refused submission left a run behind: %v", err)
	}
}

func TestDecisionsWaitForRequiredDocuments(t *testing.T) {
	server, sessionID := runnableClaim(t)

	// Fast-track review requires a witness statement when the run starts…
	workflow := seededWorkflow(t, server, sessionID, store.TypeClaim)
	base := "/api/workflows/" + workflow.DefinitionID
	fastTrack := stepCalled(t, workflow, "fast-track review")
	edit := editStep(fastTrack)
	edit.Expects = []string{"witness-statement"}
	jsonRequest(t, server, sessionID, http.MethodPatch, base+"/steps/"+fastTrack.ID, edit, http.StatusOK)

	caseRequest(t, server, sessionID, http.MethodPost, "/api/cases/C-1042/submit", http.StatusOK)
	completeAsAIStep(t, server, sessionID, "fast track")

	// …and still does after the definition stops requiring it: the run froze it.
	edit.Expects = []string{}
	jsonRequest(t, server, sessionID, http.MethodPatch, base+"/steps/"+fastTrack.ID, edit, http.StatusOK)

	subject := readCase(t, server, sessionID)
	if len(subject.CurrentStep.Required) != 1 || subject.CurrentStep.Required[0].Name != "witness-statement" ||
		subject.CurrentStep.Required[0].Present {
		t.Fatalf("required = %+v, want a missing witness statement", subject.CurrentStep.Required)
	}

	refused := requestAs(t, server, sessionID, "user:dana", http.MethodPost, "/api/cases/C-1042/decide",
		decision(t, subject, "settle"), http.StatusConflict)
	if !strings.Contains(refused.Body.String(), "Witness statement") {
		t.Errorf("refusal does not name the missing type: %s", refused.Body.String())
	}

	// Only whoever the case waits on may file on it once it is submitted.
	witness := newDocumentJSON{FileName: "statement.txt", Body: "I saw the car hit the post.", Kind: "witness-statement"}
	requestAs(t, server, sessionID, "user:marek", http.MethodPost, "/api/cases/C-1042/documents",
		witness, http.StatusForbidden)
	jsonRequest(t, server, sessionID, http.MethodPost, "/api/cases/C-1042/documents", witness, http.StatusForbidden)
	requestAs(t, server, sessionID, "user:dana", http.MethodPost, "/api/cases/C-1042/documents",
		witness, http.StatusOK)

	requestAs(t, server, sessionID, "user:dana", http.MethodPost, "/api/cases/C-1042/decide",
		decision(t, readCase(t, server, sessionID), "settle"), http.StatusOK)

	// A finished case is waiting on nobody, so nobody may file on it.
	requestAs(t, server, sessionID, "user:dana", http.MethodPost, "/api/cases/C-1042/documents",
		witness, http.StatusForbidden)
}

func TestHandingToAnAIStepChecksItsDocuments(t *testing.T) {
	server, sessionID := runnableClaim(t)

	workflow := seededWorkflow(t, server, sessionID, store.TypeClaim)
	base := "/api/workflows/" + workflow.DefinitionID

	instructions := "Check the witness statement against the account."
	response := jsonRequest(t, server, sessionID, http.MethodPost, base+"/steps", stepEditJSON{
		Name: "witness check", Assignee: "ai:witness", StatusID: workflow.Statuses[0].ID,
		Instructions: &instructions, Expects: []string{"witness-statement"},
	}, http.StatusOK)

	var updated workflowJSON
	if err := json.Unmarshal(response.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}

	jsonRequest(t, server, sessionID, http.MethodPost,
		base+"/steps/"+stepCalled(t, updated, "fast-track review").ID+"/actions",
		actionEditJSON{Name: "check witness", NextStepID: stepCalled(t, updated, "witness check").ID},
		http.StatusOK)

	caseRequest(t, server, sessionID, http.MethodPost, "/api/cases/C-1042/submit", http.StatusOK)
	completeAsAIStep(t, server, sessionID, "fast track")

	// Triage's decision documents are what it required, as they stood when it
	// decided — not the photograph, which was on file and required by nothing.
	subject := readCase(t, server, sessionID)
	kinds := make(map[string]string, len(subject.Documents))
	for _, document := range subject.Documents {
		kinds[document.ID] = document.Kind
	}

	var read []string
	for _, id := range subject.History[0].DocumentIDs {
		read = append(read, kinds[id])
	}

	slices.Sort(read)
	if want := []string{"intake-note", "police-report"}; !slices.Equal(read, want) {
		t.Errorf("triage's decision documents are %v, want %v", read, want)
	}

	// The AI step needs a witness statement; the person handing over is told so.
	refused := requestAs(t, server, sessionID, "user:dana", http.MethodPost, "/api/cases/C-1042/decide",
		decision(t, subject, "check witness"), http.StatusConflict)
	if !strings.Contains(refused.Body.String(), "witness check") ||
		!strings.Contains(refused.Body.String(), "Witness statement") {
		t.Errorf("refusal does not name the AI step and the type: %s", refused.Body.String())
	}

	// Estimate check is an AI step too, and has what it needs.
	requestAs(t, server, sessionID, "user:dana", http.MethodPost, "/api/cases/C-1042/decide",
		decision(t, subject, "escalate"), http.StatusOK)
}
