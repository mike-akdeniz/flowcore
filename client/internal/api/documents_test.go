package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mike-akdeniz/flowcore"
	"github.com/mike-akdeniz/flowcore/client/internal/app"
	"github.com/mike-akdeniz/flowcore/client/internal/samples"
	"github.com/mike-akdeniz/flowcore/client/internal/store"
)

// These tests use an already migrated CaseWork database and create their own
// session. They never reset the database or start agent workers.
func documentTestServer(t *testing.T) (*Server, string) {
	t.Helper()

	databaseURL := os.Getenv("CASEWORK_TEST_DSN")
	if databaseURL == "" {
		t.Skip("set CASEWORK_TEST_DSN to a migrated CaseWork Postgres database")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(pool.Close)

	library, err := samples.Load(os.DirFS("../../sample-documents"))
	if err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application := app.New(app.Config{}, pool, library, logger)
	sessionID := "documents-test-" + uuid.NewString()
	if _, err := application.Store.TouchSession(ctx, sessionID); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		registered, err := application.Store.RegisteredWorkflows(ctx, sessionID)
		if err != nil {
			t.Error(err)
		}

		for _, workflow := range registered {
			if err := application.Catalog.DeleteWorkflowDefinitionWithInstances(ctx, workflow.FlowcoreDefinitionID); err != nil {
				t.Error(err)
			}
		}

		if _, err := pool.Exec(ctx, `delete from casework.session where id = $1`, sessionID); err != nil {
			t.Error(err)
		}
	})

	if err := application.SeedSession(ctx, sessionID); err != nil {
		t.Fatal(err)
	}

	return NewServer(application, logger, http.NotFoundHandler()), sessionID
}

func caseRequest(t *testing.T, server *Server, sessionID, method, path string, status int) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(method, path, nil)
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: sessionID})
	response := httptest.NewRecorder()
	server.Routes().ServeHTTP(response, request)
	if response.Code != status {
		t.Fatalf("%s %s: status %d, want %d: %s", method, path, response.Code, status, response.Body.String())
	}

	return response
}

func readCase(t *testing.T, server *Server, sessionID string) caseJSON {
	t.Helper()

	response := caseRequest(t, server, sessionID, http.MethodGet, "/api/cases/C-1042", http.StatusOK)
	var subject caseJSON
	if err := json.Unmarshal(response.Body.Bytes(), &subject); err != nil {
		t.Fatal(err)
	}

	return subject
}

func TestDocumentRemovalAndHistory(t *testing.T) {
	server, sessionID := documentTestServer(t)
	ctx := context.Background()
	submission, err := server.app.Store.SubmissionByReference(ctx, sessionID, "C-1042")
	if err != nil {
		t.Fatal(err)
	}

	original := readCase(t, server, sessionID)
	for _, document := range original.Documents {
		if document.ReadBy == nil || len(document.ReadBy) != 0 || document.Version != 1 {
			t.Fatalf("new draft document has invalid readers or version: %+v", document)
		}
	}

	first := original.Documents[0]
	caseRequest(t, server, sessionID, http.MethodDelete,
		"/api/cases/P-2087/documents/"+first.ID, http.StatusConflict)
	if !reflect.DeepEqual(original.Documents, readCase(t, server, sessionID).Documents) {
		t.Fatal("another case could remove this case's document")
	}

	body := "A replacement document whose exact text must remain readable."
	firstType, err := server.app.DocumentTypeNamed(ctx, sessionID, first.Kind)
	if err != nil {
		t.Fatal(err)
	}

	add := func() store.Document {
		t.Helper()

		document, err := server.app.Store.AddDocument(ctx, store.Document{
			ID: uuid.Must(uuid.NewV7()), SubmissionID: submission.ID,
			Name: first.Name, DocumentTypeID: firstType.ID, ReceivedAt: time.Now(), Body: &body,
		})
		if err != nil {
			t.Fatal(err)
		}

		return document
	}

	replacement := add()
	superseded := readCase(t, server, sessionID)
	if !superseded.Documents[0].Superseded || superseded.Documents[len(superseded.Documents)-1].Version != 2 {
		t.Fatal("replacement did not supersede version 1 with version 2")
	}

	removePath := func(id string) string { return "/api/cases/C-1042/documents/" + id }
	caseRequest(t, server, sessionID, http.MethodDelete, removePath(replacement.ID.String()), http.StatusOK)
	restored := readCase(t, server, sessionID)
	if restored.Revision != original.Revision+2 || restored.Documents[0].Superseded {
		t.Fatal("removing an unused replacement must bump revision and restore the older document")
	}

	replacement = add()
	// Submit deliberately receives the old draft value: it must re-read the revision.
	if err := server.app.Submit(ctx, sessionID, submission); err != nil {
		t.Fatal(err)
	}

	active := readCase(t, server, sessionID)
	caseRequest(t, server, sessionID, http.MethodDelete, removePath(first.ID), http.StatusConflict)
	state, err := server.app.Engine.GetState(ctx, app.SubjectReference(sessionID, submission), activeDefinition(t, server, sessionID))
	if err != nil {
		t.Fatal(err)
	}

	if state.SubjectVersionToken == nil || *state.SubjectVersionToken != strconv.Itoa(active.Revision) {
		t.Fatal("submission stamped an obsolete revision")
	}

	// Finish through ordinary library decisions without dispatching agents.
	for i := 0; state.CurrentStep != nil && i < 20; i++ {
		action := state.CurrentStep.Actions[0]
		for _, candidate := range state.CurrentStep.Actions {
			if candidate.Name == "fast track" || candidate.Name == "settle" {
				action = candidate
				break
			}
		}

		revision := strconv.Itoa(active.Revision)
		state, err = server.app.Engine.CompleteStep(ctx, flowcore.CompleteParams{
			VisitID: state.CurrentStep.VisitID, ActionID: action.ID,
			CompletedBy: "document-test", SubjectVersionToken: &revision,
		})
		if err != nil {
			t.Fatal(err)
		}

		if state.CurrentStep != nil {
			caseRequest(t, server, sessionID, http.MethodDelete, removePath(first.ID), http.StatusConflict)
		}
	}

	if state.CurrentStep != nil {
		t.Fatal("test workflow did not finish")
	}

	caseRequest(t, server, sessionID, http.MethodDelete, removePath(first.ID), http.StatusConflict)
	caseRequest(t, server, sessionID, http.MethodPost, "/api/cases/C-1042/reopen", http.StatusOK)
	before := readCase(t, server, sessionID)
	caseRequest(t, server, sessionID, http.MethodDelete, removePath(replacement.ID.String()), http.StatusConflict)
	caseRequest(t, server, sessionID, http.MethodDelete, removePath(first.ID), http.StatusOK)
	after := readCase(t, server, sessionID)
	if !reflect.DeepEqual(before.History, after.History) || after.Revision != before.Revision+1 {
		t.Fatal("removal changed historical document sets or failed to bump revision")
	}

	for _, document := range after.Documents {
		if document.ID == replacement.ID.String() && (document.Body == nil || *document.Body != body || len(document.ReadBy) == 0) {
			t.Fatal("historical replacement lost its content or readers")
		}
	}

	// A second run must not erase the first run's protection.
	caseRequest(t, server, sessionID, http.MethodPost, "/api/cases/C-1042/submit", http.StatusOK)
	if len(readCase(t, server, sessionID).History) != len(before.History)+1 {
		t.Fatal("second run lost the previous run's history")
	}
}

func activeDefinition(t *testing.T, server *Server, sessionID string) uuid.UUID {
	t.Helper()

	workflow, err := server.app.Store.ActiveWorkflow(context.Background(), sessionID, store.TypeClaim)
	if err != nil {
		t.Fatal(err)
	}

	return workflow.FlowcoreDefinitionID
}

func TestConcurrentSubmissionAndRemoval(t *testing.T) {
	server, sessionID := documentTestServer(t)
	ctx := context.Background()
	submission, err := server.app.Store.SubmissionByReference(ctx, sessionID, "C-1042")
	if err != nil {
		t.Fatal(err)
	}

	documents, err := server.app.Store.Documents(ctx, submission.ID)
	if err != nil {
		t.Fatal(err)
	}

	// The photograph, because nothing requires it: removing a required document
	// would make the submission refuse for that reason instead, and this test is
	// about the lock between the two, not the requirement.
	photograph := slices.IndexFunc(documents, func(document store.Document) bool {
		return document.Kind == "photograph"
	})
	if photograph < 0 {
		t.Fatal("the seeded claim has no photograph")
	}

	var workers sync.WaitGroup
	var submitError, removeError error
	start := make(chan struct{})
	workers.Add(2)
	go func() {
		defer workers.Done()
		<-start
		submitError = server.app.Submit(ctx, sessionID, submission)
	}()
	go func() {
		defer workers.Done()
		<-start
		removeError = server.app.RemoveDocument(ctx, sessionID, submission, documents[photograph].ID)
	}()
	close(start)
	workers.Wait()
	if submitError != nil {
		t.Fatal(submitError)
	}

	state, err := server.app.Engine.GetState(ctx, app.SubjectReference(sessionID, submission), activeDefinition(t, server, sessionID))
	if err != nil {
		t.Fatal(err)
	}

	if removeError != nil && !errors.Is(removeError, store.ErrNotDraft) {
		t.Fatalf("unexpected removal error: %v", removeError)
	}

	expectedRevision := submission.Revision
	if removeError == nil {
		expectedRevision++
	}

	if state.SubjectVersionToken == nil || *state.SubjectVersionToken != fmt.Sprint(expectedRevision) {
		t.Fatalf("run started against wrong revision after concurrent removal: %v", state.SubjectVersionToken)
	}

	if err := server.app.RemoveDocument(ctx, sessionID, submission, documents[1].ID); err == nil {
		t.Fatal("stale draft allowed deletion after submission")
	}
}

func TestOpenRunProtectsDocumentsAfterPartialSubmission(t *testing.T) {
	server, sessionID := documentTestServer(t)
	ctx := context.Background()
	submission, err := server.app.Store.SubmissionByReference(ctx, sessionID, "C-1042")
	if err != nil {
		t.Fatal(err)
	}

	// Reproduce a successful library Start followed by a failed client status write.
	revision := strconv.Itoa(submission.Revision)
	_, err = server.app.Engine.Start(ctx, flowcore.StartParams{
		WorkflowDefinitionID: activeDefinition(t, server, sessionID),
		SubjectReference:     app.SubjectReference(sessionID, submission),
		SubjectVersionToken:  &revision,
	})
	if err != nil {
		t.Fatal(err)
	}

	documents, err := server.app.Store.Documents(ctx, submission.ID)
	if err != nil {
		t.Fatal(err)
	}

	caseRequest(t, server, sessionID, http.MethodDelete,
		"/api/cases/C-1042/documents/"+documents[0].ID.String(), http.StatusConflict)
}
