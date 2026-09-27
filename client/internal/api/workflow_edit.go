package api

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/mike-akdeniz/flowcore/client/internal/app"
	"github.com/mike-akdeniz/flowcore/client/internal/store"
)

// Editing a workflow.
//
// Every handler here answers with the whole workflow rather than with whatever
// it changed, for the same reason the case handlers do: one edit moves several
// things at once. Adding an action can make an unreachable step reachable and
// clear a concern; deleting a step can strand two others. A response carrying
// only the new row would leave the browser to work that out, and it would work
// it out differently from the server.
//
// Ownership is checked in the app layer, on every call. FlowCore will happily
// return or modify any definition whose id you name, because it has no tenant —
// refusing another session's is entirely CaseWork's job, and the place to do it
// is the layer that knows what a session is.

func (s *Server) editWorkflow(
	w http.ResponseWriter,
	r *http.Request,
	edit func(sessionID string, definitionID uuid.UUID) error,
) {
	definitionID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)

		return
	}

	sessionID := sessionFrom(r)

	if err := edit(sessionID, definitionID); err != nil {
		s.failEdit(w, err)

		return
	}

	s.writeWorkflow(w, r, sessionID, definitionID)
}

// failEdit turns the app layer's errors into something a form can show.
//
// ErrNotYours is a 404 rather than a 403 on purpose: a session should not be
// able to learn that another session's workflow exists by being told it is
// forbidden.
func (s *Server) failEdit(w http.ResponseWriter, err error) {
	if err == app.ErrNotYours {
		http.Error(w, "no such workflow", http.StatusNotFound)

		return
	}

	// Everything else is the library refusing something: a duplicate name, an
	// action that neither routes nor terminates, a status still in use. Those
	// messages are written for a caller and are the most useful thing to show.
	http.Error(w, err.Error(), http.StatusBadRequest)
}

type nameJSON struct {
	Name string `json:"name"`
}

type stepEditJSON struct {
	Name     string `json:"name"`
	Assignee string `json:"assignee"`
	StatusID string `json:"statusId"`
	// Expects is the whole set of document types this step reads, not a delta.
	// Sending the set means the browser never has to work out which attachments
	// to add and which to remove.
	Expects []string `json:"expects"`
}

type actionEditJSON struct {
	Name string `json:"name"`
	// Exactly one of these. The schema enforces it too, so a request setting
	// both or neither is refused by the library with its own message.
	NextStepID       string `json:"nextStepId"`
	TerminalStatusID string `json:"terminalStatusId"`
}

func (s *Server) renameWorkflow(w http.ResponseWriter, r *http.Request) {
	var body nameJSON
	if !decode(w, r, &body) {
		return
	}

	s.editWorkflow(w, r, func(sessionID string, definitionID uuid.UUID) error {
		return s.app.RenameDefinition(r.Context(), sessionID, definitionID, body.Name)
	})
}

func (s *Server) addStatus(w http.ResponseWriter, r *http.Request) {
	var body nameJSON
	if !decode(w, r, &body) {
		return
	}

	s.editWorkflow(w, r, func(sessionID string, definitionID uuid.UUID) error {
		return s.app.AddStatus(r.Context(), sessionID, definitionID, body.Name)
	})
}

func (s *Server) updateStatus(w http.ResponseWriter, r *http.Request) {
	var body nameJSON
	if !decode(w, r, &body) {
		return
	}

	statusID, ok := pathID(w, r, "statusId")
	if !ok {
		return
	}

	s.editWorkflow(w, r, func(sessionID string, definitionID uuid.UUID) error {
		return s.app.UpdateStatus(r.Context(), sessionID, definitionID, statusID, body.Name)
	})
}

func (s *Server) deleteStatus(w http.ResponseWriter, r *http.Request) {
	statusID, ok := pathID(w, r, "statusId")
	if !ok {
		return
	}

	s.editWorkflow(w, r, func(sessionID string, definitionID uuid.UUID) error {
		return s.app.DeleteStatus(r.Context(), sessionID, definitionID, statusID)
	})
}

func (s *Server) addStep(w http.ResponseWriter, r *http.Request) {
	var body stepEditJSON
	if !decode(w, r, &body) {
		return
	}

	statusID, err := uuid.Parse(body.StatusID)
	if err != nil {
		http.Error(w, "a step needs a status", http.StatusBadRequest)

		return
	}

	s.editWorkflow(w, r, func(sessionID string, definitionID uuid.UUID) error {
		return s.app.AddStepWithTypes(r.Context(), sessionID, definitionID, app.AddStepRequest{
			Name:       body.Name,
			StatusID:   statusID,
			AssigneeID: body.Assignee,
		}, body.Expects)
	})
}

func (s *Server) updateStep(w http.ResponseWriter, r *http.Request) {
	var body stepEditJSON
	if !decode(w, r, &body) {
		return
	}

	stepID, ok := pathID(w, r, "stepId")
	if !ok {
		return
	}

	statusID, err := uuid.Parse(body.StatusID)
	if err != nil {
		http.Error(w, "a step needs a status", http.StatusBadRequest)

		return
	}

	s.editWorkflow(w, r, func(sessionID string, definitionID uuid.UUID) error {
		return s.app.UpdateStepWithTypes(r.Context(), sessionID, definitionID, stepID,
			app.AddStepRequest{
				Name:       body.Name,
				StatusID:   statusID,
				AssigneeID: body.Assignee,
			}, body.Expects)
	})
}

func (s *Server) deleteStep(w http.ResponseWriter, r *http.Request) {
	stepID, ok := pathID(w, r, "stepId")
	if !ok {
		return
	}

	s.editWorkflow(w, r, func(sessionID string, definitionID uuid.UUID) error {
		return s.app.DeleteStep(r.Context(), sessionID, definitionID, stepID)
	})
}

func (s *Server) setEntryStep(w http.ResponseWriter, r *http.Request) {
	stepID, ok := pathID(w, r, "stepId")
	if !ok {
		return
	}

	s.editWorkflow(w, r, func(sessionID string, definitionID uuid.UUID) error {
		return s.app.SetEntryStep(r.Context(), sessionID, definitionID, stepID)
	})
}

func (s *Server) addAction(w http.ResponseWriter, r *http.Request) {
	var body actionEditJSON
	if !decode(w, r, &body) {
		return
	}

	stepID, ok := pathID(w, r, "stepId")
	if !ok {
		return
	}

	request := app.AddActionRequest{Name: body.Name}

	if body.NextStepID != "" {
		next, err := uuid.Parse(body.NextStepID)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)

			return
		}

		request.NextStepID = &next
	}

	if body.TerminalStatusID != "" {
		terminal, err := uuid.Parse(body.TerminalStatusID)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)

			return
		}

		request.TerminalStatusID = &terminal
	}

	s.editWorkflow(w, r, func(sessionID string, definitionID uuid.UUID) error {
		return s.app.AddAction(r.Context(), sessionID, definitionID, stepID, request)
	})
}

func (s *Server) updateAction(w http.ResponseWriter, r *http.Request) {
	var body nameJSON
	if !decode(w, r, &body) {
		return
	}

	actionID, ok := pathID(w, r, "actionId")
	if !ok {
		return
	}

	s.editWorkflow(w, r, func(sessionID string, definitionID uuid.UUID) error {
		return s.app.UpdateAction(r.Context(), sessionID, definitionID, actionID, body.Name)
	})
}

func (s *Server) deleteAction(w http.ResponseWriter, r *http.Request) {
	actionID, ok := pathID(w, r, "actionId")
	if !ok {
		return
	}

	s.editWorkflow(w, r, func(sessionID string, definitionID uuid.UUID) error {
		return s.app.DeleteAction(r.Context(), sessionID, definitionID, actionID)
	})
}

type activateJSON struct {
	SubmissionType string `json:"submissionType"`
}

// activateWorkflow makes this the workflow new submissions of a type will run.
//
// It does not check the concerns first. They are on the payload the editor
// already has, so a visitor activating a workflow that cannot finish has been
// told; taking the decision away would invent a rule the library declines to
// have, and freeze one definition of "sound" into code.
func (s *Server) activateWorkflow(w http.ResponseWriter, r *http.Request) {
	var body activateJSON
	if !decode(w, r, &body) {
		return
	}

	submissionType := store.SubmissionType(body.SubmissionType)
	if submissionType != store.TypeClaim && submissionType != store.TypeApplication {
		http.Error(w, "unknown submission type", http.StatusBadRequest)

		return
	}

	s.editWorkflow(w, r, func(sessionID string, definitionID uuid.UUID) error {
		return s.app.Activate(r.Context(), sessionID, definitionID, submissionType)
	})
}

// --- helpers ----------------------------------------------------------------

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(r.Body).Decode(into); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)

		return false
	}

	return true
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		http.NotFound(w, r)

		return uuid.Nil, false
	}

	return id, true
}

// --- creating workflows and document types ----------------------------------

type newWorkflowJSON struct {
	Name string `json:"name"`
	// What it is for. A workflow is registered against a submission type when it
	// is created, inactive, and activated separately.
	SubmissionType string `json:"submissionType"`
	// A definition cannot be created empty — the library refuses it — so the
	// first step and the first status come with it. Three fields is the smallest
	// honest form.
	StatusName string `json:"statusName"`
	StepName   string `json:"stepName"`
	Assignee   string `json:"assignee"`
}

func (s *Server) createWorkflow(w http.ResponseWriter, r *http.Request) {
	var body newWorkflowJSON
	if !decode(w, r, &body) {
		return
	}

	submissionType := store.SubmissionType(body.SubmissionType)
	if submissionType != store.TypeClaim && submissionType != store.TypeApplication {
		http.Error(w, "unknown submission type", http.StatusBadRequest)

		return
	}

	definition, err := s.app.CreateDefinition(r.Context(), sessionFrom(r), app.NewDefinition{
		Name:           body.Name,
		SubmissionType: submissionType,
		StatusName:     body.StatusName,
		StepName:       body.StepName,
		AssigneeID:     body.Assignee,
	})
	if err != nil {
		s.failEdit(w, err)

		return
	}

	s.writeWorkflow(w, r, sessionFrom(r), definition.ID)
}

type documentTypeEditJSON struct {
	Name  string `json:"name"`
	Title string `json:"title"`
}

func (s *Server) listDocumentTypes(w http.ResponseWriter, r *http.Request) {
	types, err := s.app.Store.DocumentTypes(r.Context(), sessionFrom(r))
	if err != nil {
		s.fail(w, "could not read the document types", err)

		return
	}

	payload := make([]documentTypeJSON, 0, len(types))
	for _, documentType := range types {
		payload = append(payload,
			documentTypeJSON{Name: documentType.Name, Title: documentType.Title})
	}

	s.write(w, payload)
}

// createDocumentType makes a kind of document, from the step that needs one.
//
// No findings. They are what a simulated agent step says about a document of
// this kind, and `finding()` already falls back to generic wording when they are
// blank — so a type made in thirty seconds behaves sensibly, and writing proper
// wording is an improvement rather than a prerequisite.
func (s *Server) createDocumentType(w http.ResponseWriter, r *http.Request) {
	var body documentTypeEditJSON
	if !decode(w, r, &body) {
		return
	}

	created, err := s.app.CreateDocumentType(r.Context(), sessionFrom(r), body.Name, body.Title)
	if err != nil {
		s.failEdit(w, err)

		return
	}

	s.write(w, documentTypeJSON{Name: created.Name, Title: created.Title})
}
