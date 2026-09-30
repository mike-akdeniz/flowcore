package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/mike-akdeniz/flowcore"
	"github.com/mike-akdeniz/flowcore/client/internal/store"
)

// Tests for the document type rules of client decisions 36 to 38: what a kind of
// case may hold, what a step may require, and when either may change. They run
// against the seeded session, whose requirements live on the FlowCore
// definitions as type ids.

func jsonRequest(t *testing.T, server *Server, sessionID, method, path string, body any, status int) *httptest.ResponseRecorder {
	t.Helper()

	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: sessionID})
	response := httptest.NewRecorder()
	server.Routes().ServeHTTP(response, request)
	if response.Code != status {
		t.Fatalf("%s %s: status %d, want %d: %s", method, path, response.Code, status, response.Body.String())
	}

	return response
}

// seededWorkflow reads the registered workflow for a kind of case, as the editor
// sees it.
func seededWorkflow(t *testing.T, server *Server, sessionID string, submissionType store.SubmissionType) workflowJSON {
	t.Helper()

	workflow, err := server.app.Store.ActiveWorkflow(context.Background(), sessionID, submissionType)
	if err != nil {
		t.Fatal(err)
	}

	response := caseRequest(t, server, sessionID, http.MethodGet,
		"/api/workflows/"+workflow.FlowcoreDefinitionID.String(), http.StatusOK)

	var payload workflowJSON
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}

	return payload
}

func stepCalled(t *testing.T, workflow workflowJSON, name string) stepJSON {
	t.Helper()

	for _, step := range workflow.Steps {
		if step.Name == name {
			return step
		}
	}

	t.Fatalf("no step %q", name)

	return stepJSON{}
}

func editStep(step stepJSON) stepEditJSON {
	return stepEditJSON{Name: step.Name, Assignee: step.Assignee, StatusID: step.StatusID, Expects: step.Expects}
}

func TestSeededDocumentTypes(t *testing.T) {
	server, sessionID := documentTestServer(t)

	claim := readCase(t, server, sessionID)
	slices.Sort(claim.Expects)
	if want := []string{"correspondence", "estimate", "intake-note", "photograph", "police-report",
		"witness-statement"}; !slices.Equal(claim.Expects, want) {
		t.Errorf("a claim may hold %v, want its whole allowed list %v", claim.Expects, want)
	}

	workflow := seededWorkflow(t, server, sessionID, store.TypeClaim)
	triage := stepCalled(t, workflow, "triage")
	slices.Sort(triage.Expects)
	if want := []string{"estimate", "intake-note", "police-report"}; !slices.Equal(triage.Expects, want) {
		t.Errorf("triage requires %v, want %v", triage.Expects, want)
	}

	if triage.Instructions == nil || *triage.Instructions == "" {
		t.Error("the seeded agent step has no instructions")
	}

	if adjuster := stepCalled(t, workflow, "adjuster review"); len(adjuster.Expects) != 0 {
		t.Errorf("adjuster review requires %v, want nothing", adjuster.Expects)
	}
}

func TestDocumentsMustBeAllowedOnTheCase(t *testing.T) {
	server, sessionID := documentTestServer(t)

	// An inspection is an application's document, not a claim's.
	jsonRequest(t, server, sessionID, http.MethodPost, "/api/cases/C-1042/documents",
		newDocumentJSON{SampleFile: "3-inspection-pass.txt"}, http.StatusBadRequest)
	jsonRequest(t, server, sessionID, http.MethodPost, "/api/cases/C-1042/documents",
		newDocumentJSON{FileName: "notes.txt", Body: "text", Kind: "inspection"}, http.StatusBadRequest)

	jsonRequest(t, server, sessionID, http.MethodPost, "/api/cases/C-1042/documents",
		newDocumentJSON{FileName: "letter.txt", Body: "text", Kind: "correspondence"}, http.StatusOK)
}

func TestStepEditRules(t *testing.T) {
	server, sessionID := documentTestServer(t)

	workflow := seededWorkflow(t, server, sessionID, store.TypeClaim)
	base := "/api/workflows/" + workflow.DefinitionID

	t.Run("a step may require only what the case allows", func(t *testing.T) {
		edit := editStep(stepCalled(t, workflow, "adjuster review"))
		edit.Expects = []string{"inspection"}
		response := jsonRequest(t, server, sessionID, http.MethodPatch,
			base+"/steps/"+stepCalled(t, workflow, "adjuster review").ID, edit, http.StatusBadRequest)
		if !strings.Contains(response.Body.String(), "inspection") {
			t.Errorf("error does not name the type: %s", response.Body.String())
		}
	})

	t.Run("an agent step needs instructions", func(t *testing.T) {
		jsonRequest(t, server, sessionID, http.MethodPost, base+"/steps", stepEditJSON{
			Name: "photo check", Assignee: "agent:photos", StatusID: workflow.Statuses[0].ID,
		}, http.StatusBadRequest)
	})

	t.Run("an edit that does not send instructions keeps them", func(t *testing.T) {
		triage := stepCalled(t, workflow, "triage")
		edit := editStep(triage)
		edit.Name = "claim triage"
		response := jsonRequest(t, server, sessionID, http.MethodPatch, base+"/steps/"+triage.ID, edit, http.StatusOK)

		var updated workflowJSON
		if err := json.Unmarshal(response.Body.Bytes(), &updated); err != nil {
			t.Fatal(err)
		}

		renamed := stepCalled(t, updated, "claim triage")
		if renamed.Instructions == nil || *renamed.Instructions != *triage.Instructions {
			t.Errorf("instructions = %v, want them kept", renamed.Instructions)
		}
	})

	t.Run("an agent may hand over only what it required", func(t *testing.T) {
		instructions := "Check the witness statement."
		response := jsonRequest(t, server, sessionID, http.MethodPost, base+"/steps", stepEditJSON{
			Name: "witness check", Assignee: "agent:witness", StatusID: workflow.Statuses[0].ID,
			Instructions: &instructions, Expects: []string{"witness-statement"},
		}, http.StatusOK)

		var updated workflowJSON
		if err := json.Unmarshal(response.Body.Bytes(), &updated); err != nil {
			t.Fatal(err)
		}

		witness := stepCalled(t, updated, "witness check")
		documentation := stepCalled(t, updated, "estimate check")

		refused := jsonRequest(t, server, sessionID, http.MethodPost,
			base+"/steps/"+documentation.ID+"/actions",
			actionEditJSON{Name: "check witness", NextStepID: witness.ID}, http.StatusBadRequest)
		if !strings.Contains(refused.Body.String(), "Witness statement") {
			t.Errorf("error does not name the missing type: %s", refused.Body.String())
		}

		// A person may hand it over: they can file the statement first.
		jsonRequest(t, server, sessionID, http.MethodPost,
			base+"/steps/"+stepCalled(t, updated, "adjuster review").ID+"/actions",
			actionEditJSON{Name: "check witness", NextStepID: witness.ID}, http.StatusOK)

		// Once the destination needs nothing the source lacks, the agent may too.
		edit := editStep(witness)
		edit.Expects = []string{"police-report"}
		jsonRequest(t, server, sessionID, http.MethodPatch, base+"/steps/"+witness.ID, edit, http.StatusOK)
		jsonRequest(t, server, sessionID, http.MethodPost,
			base+"/steps/"+documentation.ID+"/actions",
			actionEditJSON{Name: "check witness", NextStepID: witness.ID}, http.StatusOK)

		// And an edit that would break an existing handoff is refused too.
		edit.Expects = []string{"witness-statement"}
		jsonRequest(t, server, sessionID, http.MethodPatch, base+"/steps/"+witness.ID, edit, http.StatusBadRequest)
	})
}

func TestAllowedListRemoval(t *testing.T) {
	server, sessionID := documentTestServer(t)
	ctx := context.Background()
	path := "/api/case-types/claim/document-types"

	claimTypes := []string{"correspondence", "estimate", "intake-note", "photograph", "police-report",
		"witness-statement"}
	without := func(name string) []string {
		return slices.DeleteFunc(slices.Clone(claimTypes), func(candidate string) bool { return candidate == name })
	}

	// Required by the registered definition.
	jsonRequest(t, server, sessionID, http.MethodPut, path,
		allowedDocumentTypesJSON{Names: without("police-report")}, http.StatusBadRequest)

	// Required by nothing.
	jsonRequest(t, server, sessionID, http.MethodPut, path,
		allowedDocumentTypesJSON{Names: without("witness-statement")}, http.StatusOK)
	claimTypes = without("witness-statement")

	// Start a run while police reports are required, then remove every
	// requirement from the definition: the open run still demands one.
	caseRequest(t, server, sessionID, http.MethodPost, "/api/cases/C-1042/submit", http.StatusOK)

	workflow := seededWorkflow(t, server, sessionID, store.TypeClaim)
	base := "/api/workflows/" + workflow.DefinitionID
	for _, name := range []string{"narrative consistency", "estimate check", "triage"} {
		step := stepCalled(t, workflow, name)
		edit := editStep(step)
		edit.Expects = slices.DeleteFunc(slices.Clone(step.Expects),
			func(candidate string) bool { return candidate == "police-report" })
		jsonRequest(t, server, sessionID, http.MethodPatch, base+"/steps/"+step.ID, edit, http.StatusOK)
	}

	refused := jsonRequest(t, server, sessionID, http.MethodPut, path,
		allowedDocumentTypesJSON{Names: without("police-report")}, http.StatusBadRequest)
	if !strings.Contains(refused.Body.String(), "C-1042") {
		t.Errorf("error does not name the case still requiring it: %s", refused.Body.String())
	}

	// A finished run does not block it. Finish C-1042's run by the fast track.
	submission, err := server.app.Store.SubmissionByReference(ctx, sessionID, "C-1042")
	if err != nil {
		t.Fatal(err)
	}

	for _, action := range []string{"fast track", "settle"} {
		state, err := server.app.Engine.GetState(ctx, *submission.SubjectReference, *submission.FlowcoreDefinitionID)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := server.app.Engine.CompleteStep(ctx, flowcore.CompleteParams{
			VisitID:     state.CurrentStep.VisitID,
			ActionID:    actionCalled(t, state, action),
			CompletedBy: "test",
		}); err != nil {
			t.Fatal(err)
		}
	}

	jsonRequest(t, server, sessionID, http.MethodPut, path,
		allowedDocumentTypesJSON{Names: without("police-report")}, http.StatusOK)
}

func TestRetitleKeepsIdentity(t *testing.T) {
	server, sessionID := documentTestServer(t)

	before := readCase(t, server, sessionID)
	jsonRequest(t, server, sessionID, http.MethodPatch, "/api/document-types/estimate",
		documentTypeEditJSON{Title: "Garage quote"}, http.StatusOK)
	after := readCase(t, server, sessionID)

	var title string
	for _, documentType := range after.DocumentTypes {
		if documentType.Name == "estimate" {
			title = documentType.Title
		}
	}

	if title != "Garage quote" {
		t.Errorf("estimate is titled %q, want the new title", title)
	}

	// The filed document keeps its own name and its type.
	for i, document := range before.Documents {
		if after.Documents[i].Name != document.Name || after.Documents[i].Kind != document.Kind {
			t.Errorf("document %d changed from %s/%s to %s/%s", i,
				document.Name, document.Kind, after.Documents[i].Name, after.Documents[i].Kind)
		}
	}

	workflow := seededWorkflow(t, server, sessionID, store.TypeClaim)
	if !slices.Contains(stepCalled(t, workflow, "triage").Expects, "estimate") {
		t.Error("triage no longer requires the retitled type")
	}
}

func actionCalled(t *testing.T, state flowcore.WorkflowState, name string) uuid.UUID {
	t.Helper()

	for _, action := range state.CurrentStep.Actions {
		if action.Name == name {
			return action.ID
		}
	}

	t.Fatalf("no action %q on %q", name, state.CurrentStep.Name)

	return uuid.Nil
}

// Renaming an action keeps where it leads. FlowCore's update is a full replace,
// and a rename that sent only the name was refused for routing nowhere.
func TestRenameActionKeepsItsTarget(t *testing.T) {
	server, sessionID := documentTestServer(t)

	workflow := seededWorkflow(t, server, sessionID, store.TypeClaim)
	triage := stepCalled(t, workflow, "triage")
	action := triage.Actions[0]

	response := jsonRequest(t, server, sessionID, http.MethodPatch,
		"/api/workflows/"+workflow.DefinitionID+"/actions/"+action.ID,
		nameJSON{Name: "renamed"}, http.StatusOK)

	var updated workflowJSON
	if err := json.Unmarshal(response.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}

	for _, candidate := range stepCalled(t, updated, "triage").Actions {
		if candidate.ID != action.ID {
			continue
		}

		if candidate.Name != "renamed" || candidate.NextStepID == nil || *candidate.NextStepID != *action.NextStepID {
			t.Errorf("renamed action = %+v, want the new name and the same target as %+v", candidate, action)
		}

		return
	}

	t.Fatal("the renamed action is gone")
}
