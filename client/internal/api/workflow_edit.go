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
	// Expects is the whole set of document types this step requires, not a
	// delta. Sending the set means the browser never has to work out which to add
	// and which to remove.
	Expects []string `json:"expects"`
	// Instructions are left as they are when absent, and cleared by an empty
	// string, so an editor that does not show them cannot erase them.
	Instructions *string `json:"instructions"`
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
		return s.app.AddStep(r.Context(), sessionID, definitionID, app.AddStepRequest{
			Name:                  body.Name,
			StatusID:              statusID,
			AssigneeID:            body.Assignee,
			Instructions:          body.Instructions,
			RequiredDocumentTypes: body.Expects,
		})
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
		return s.app.UpdateStep(r.Context(), sessionID, definitionID, stepID,
			app.AddStepRequest{
				Name:                  body.Name,
				StatusID:              statusID,
				AssigneeID:            body.Assignee,
				Instructions:          body.Instructions,
				RequiredDocumentTypes: body.Expects,
			})
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
	// StepInstructions are required when the first step is an agent's.
	StepInstructions *string `json:"stepInstructions"`
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
		Name:             body.Name,
		SubmissionType:   submissionType,
		StatusName:       body.StatusName,
		StepName:         body.StepName,
		AssigneeID:       body.Assignee,
		StepInstructions: body.StepInstructions,
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
	// SubmissionType, when set, allows the new type on that kind of case, which
	// is what lets a step of a workflow for it require the type.
	SubmissionType string `json:"submissionType"`
}

// documentTypeListJSON is a type with the kinds of case that may hold it.
type documentTypeListJSON struct {
	Name       string   `json:"name"`
	Title      string   `json:"title"`
	AllowedFor []string `json:"allowedFor"`
}

func (s *Server) listDocumentTypes(w http.ResponseWriter, r *http.Request) {
	sessionID := sessionFrom(r)

	types, err := s.app.Store.DocumentTypes(r.Context(), sessionID)
	if err != nil {
		s.fail(w, "could not read the document types", err)

		return
	}

	allowedFor := make(map[string][]string, len(types))
	for _, submissionType := range []store.SubmissionType{store.TypeClaim, store.TypeApplication} {
		names, err := s.app.AllowedDocumentTypeNames(r.Context(), sessionID, submissionType)
		if err != nil {
			s.fail(w, "could not read the document types", err)

			return
		}

		for _, name := range names {
			allowedFor[name] = append(allowedFor[name], string(submissionType))
		}
	}

	payload := make([]documentTypeListJSON, 0, len(types))
	for _, documentType := range types {
		allowed := allowedFor[documentType.Name]
		if allowed == nil {
			allowed = []string{}
		}

		payload = append(payload, documentTypeListJSON{
			Name:       documentType.Name,
			Title:      documentType.Title,
			AllowedFor: allowed,
		})
	}

	s.write(w, payload)
}

// createDocumentType makes a kind of document, from the step that needs one.
func (s *Server) createDocumentType(w http.ResponseWriter, r *http.Request) {
	var body documentTypeEditJSON
	if !decode(w, r, &body) {
		return
	}

	var allowOn *store.SubmissionType

	if body.SubmissionType != "" {
		submissionType, ok := submissionTypeFrom(w, body.SubmissionType)
		if !ok {
			return
		}

		allowOn = &submissionType
	}

	created, err := s.app.CreateDocumentType(r.Context(), sessionFrom(r), body.Name, body.Title, allowOn)
	if err != nil {
		s.failEdit(w, err)

		return
	}

	s.write(w, documentTypeJSON{Name: created.Name, Title: created.Title})
}

// retitleDocumentType changes what a type is called. Everything that refers to
// the type does so by id, so nothing else moves.
func (s *Server) retitleDocumentType(w http.ResponseWriter, r *http.Request) {
	var body documentTypeEditJSON
	if !decode(w, r, &body) {
		return
	}

	if err := s.app.RetitleDocumentType(r.Context(), sessionFrom(r), r.PathValue("name"), body.Title); err != nil {
		s.failEdit(w, err)

		return
	}

	s.listDocumentTypes(w, r)
}

type allowedDocumentTypesJSON struct {
	// Names is the whole allowed list for the kind of case, replacing the stored
	// one, as a step's required types are sent.
	Names []string `json:"names"`
}

// setAllowedDocumentTypes replaces what a kind of case may hold. Removing a type
// something could still require is refused with the reason.
func (s *Server) setAllowedDocumentTypes(w http.ResponseWriter, r *http.Request) {
	submissionType, ok := submissionTypeFrom(w, r.PathValue("type"))
	if !ok {
		return
	}

	var body allowedDocumentTypesJSON
	if !decode(w, r, &body) {
		return
	}

	if err := s.app.SetAllowedDocumentTypes(r.Context(), sessionFrom(r), submissionType, body.Names); err != nil {
		s.failEdit(w, err)

		return
	}

	s.listDocumentTypes(w, r)
}

func submissionTypeFrom(w http.ResponseWriter, value string) (store.SubmissionType, bool) {
	submissionType := store.SubmissionType(value)
	if submissionType != store.TypeClaim && submissionType != store.TypeApplication {
		http.Error(w, "unknown submission type", http.StatusBadRequest)

		return "", false
	}

	return submissionType, true
}
