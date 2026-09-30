package api

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/mike-akdeniz/flowcore"
	"github.com/mike-akdeniz/flowcore/client/internal/app"
)

// The workflow endpoints are the one place this API speaks FlowCore's vocabulary
// rather than CaseWork's.
//
// Decision 13 carves them out deliberately: every other screen is about a claim
// or an application, but these screens genuinely are about definitions, steps and
// actions, so translating them into some other language would only obscure what
// the user is looking at.
//
// What is still CaseWork's: which workflow serves which kind of submission,
// and which is active. FlowCore has no opinion about either.

type workflowSummaryJSON struct {
	DefinitionID   string `json:"definitionId"`
	Name           string `json:"name"`
	SubmissionType string `json:"submissionType"`
	Active         bool   `json:"active"`
	StepCount      int    `json:"stepCount"`
}

func (s *Server) listWorkflows(w http.ResponseWriter, r *http.Request) {
	registered, err := s.app.Store.RegisteredWorkflows(r.Context(), sessionFrom(r))
	if err != nil {
		s.fail(w, "could not read your workflows", err)

		return
	}

	payload := make([]workflowSummaryJSON, 0, len(registered))
	for _, workflow := range registered {
		definition, err := s.app.Catalog.Get(r.Context(), workflow.FlowcoreDefinitionID)
		if err != nil {
			s.fail(w, "could not read a workflow", err)

			return
		}

		payload = append(payload, workflowSummaryJSON{
			DefinitionID:   workflow.FlowcoreDefinitionID.String(),
			Name:           workflow.Name,
			SubmissionType: string(workflow.SubmissionType),
			Active:         workflow.Active,
			StepCount:      len(definition.Steps),
		})
	}

	s.write(w, payload)
}

type actionJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Exactly one of these is set, which the library's schema enforces: an action
	// either routes to another step or ends the run in a terminal status.
	NextStepID       *string `json:"nextStepId"`
	TerminalStatusID *string `json:"terminalStatusId"`
}

type stepJSON struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Assignee string `json:"assignee"`
	StatusID string `json:"statusId"`
	IsAgent  bool   `json:"isAgent"`
	// Expects names the document types a decision on this step requires. FlowCore
	// stores them as opaque ids on the step; the names are CaseWork's.
	Expects []string `json:"expects"`
	// Instructions are what whoever acts on this step is told. Nil when none.
	Instructions *string      `json:"instructions"`
	Actions      []actionJSON `json:"actions"`
}

// concernJSON is something wrong with the graph's shape. StepID is empty when the
// concern is about the workflow rather than one step.
type concernJSON struct {
	StepID  string `json:"stepId"`
	Message string `json:"message"`
}

type statusJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type workflowJSON struct {
	DefinitionID   string       `json:"definitionId"`
	Name           string       `json:"name"`
	SubmissionType string       `json:"submissionType"`
	Active         bool         `json:"active"`
	EntryStepID    string       `json:"entryStepId"`
	Statuses       []statusJSON `json:"statuses"`
	Steps          []stepJSON   `json:"steps"`
	// Concerns are warnings about the shape, never refusals.
	Concerns []concernJSON `json:"concerns"`
	// RunningCases is how many cases are part-way through this workflow. They
	// keep the version they started on, and saying so is the only way that
	// guarantee is visible rather than merely documented.
	RunningCases int `json:"runningCases"`
}

// writeWorkflow composes and sends one workflow. Every editing handler answers
// with this, so the browser never has to work out what an edit changed.
func (s *Server) writeWorkflow(
	w http.ResponseWriter,
	r *http.Request,
	sessionID string,
	definitionID uuid.UUID,
) {
	payload, err := s.composeWorkflow(r, sessionID, definitionID)
	if err != nil {
		http.Error(w, "no such workflow", http.StatusNotFound)

		return
	}

	s.write(w, payload)
}

func (s *Server) showWorkflow(w http.ResponseWriter, r *http.Request) {
	definitionID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)

		return
	}

	s.writeWorkflow(w, r, sessionFrom(r), definitionID)
}

// composeWorkflow builds the whole workflow a screen needs: the definition from
// the library, plus the facts that are CaseWork's own — which submission type it
// serves, whether it is live, what each step expects, what is wrong with its
// shape, and how many cases are running on it.
func (s *Server) composeWorkflow(
	r *http.Request,
	sessionID string,
	definitionID uuid.UUID,
) (workflowJSON, error) {
	// Definition checks ownership before reading: FlowCore will return any
	// definition whose id you name, because it has no tenant. Refusing another
	// session's is CaseWork's job.
	definition, err := s.app.Definition(r.Context(), sessionID, definitionID)
	if err != nil {
		return workflowJSON{}, err
	}

	payload := workflowJSON{
		DefinitionID: definition.ID.String(),
		Name:         definition.Name,
		Statuses:     make([]statusJSON, 0, len(definition.Statuses)),
		Steps:        make([]stepJSON, 0, len(definition.Steps)),
		Concerns:     make([]concernJSON, 0),
	}

	payload.RunningCases, err = s.app.RunningCases(r.Context(), sessionID, definitionID)
	if err != nil {
		return workflowJSON{}, err
	}

	if definition.InitialStepDefinitionID != nil {
		payload.EntryStepID = definition.InitialStepDefinitionID.String()
	}

	// Which submission type this serves, and whether it is live, are CaseWork's
	// own facts — they live in the registry, not in the definition.
	registered, err := s.app.Store.RegisteredWorkflows(r.Context(), sessionID)
	if err != nil {
		return workflowJSON{}, err
	}

	for _, workflow := range registered {
		if workflow.FlowcoreDefinitionID == definitionID {
			payload.SubmissionType = string(workflow.SubmissionType)
			payload.Active = workflow.Active
		}
	}

	for _, status := range definition.Statuses {
		payload.Statuses = append(payload.Statuses, statusJSON{
			ID: status.ID.String(), Name: status.Name,
		})
	}

	// What is wrong with the shape, if anything. Warnings rather than refusals:
	// FlowCore has no opinion on whether a graph can finish, and a definition
	// being edited has to be allowed to be incoherent.
	for _, concern := range app.Concerns(definition) {
		entry := concernJSON{Message: concern.Message}
		if concern.StepID != uuid.Nil {
			entry.StepID = concern.StepID.String()
		}

		payload.Concerns = append(payload.Concerns, entry)
	}

	documentTypes, err := s.app.Store.DocumentTypes(r.Context(), sessionID)
	if err != nil {
		return workflowJSON{}, err
	}

	names := make(map[string]string, len(documentTypes))
	for _, documentType := range documentTypes {
		names[documentType.ID.String()] = documentType.Name
	}

	for _, step := range definition.Steps {
		expects := make([]string, 0, len(step.RequiredInputTypeIDs))
		for _, id := range step.RequiredInputTypeIDs {
			if name, ok := names[id]; ok {
				expects = append(expects, name)
			}
		}

		payload.Steps = append(payload.Steps, stepJSON{
			ID:       step.ID.String(),
			Name:     step.Name,
			Assignee: step.AssigneeID,
			StatusID: step.WorkflowStatusDefinitionID.String(),
			// Whether an assignee names an agent is CaseWork's convention, not
			// the library's — FlowCore stores "agent:triage" exactly as it stores
			// "group:claims-adjusters". Deciding it here keeps that convention in
			// one place rather than duplicated in the browser.
			IsAgent:      app.IsAgent(step.AssigneeID),
			Expects:      expects,
			Instructions: step.Instructions,
			Actions:      toActionsJSON(step.Actions),
		})
	}

	return payload, nil
}

func toActionsJSON(actions []flowcore.ActionDefinition) []actionJSON {
	payload := make([]actionJSON, 0, len(actions))

	for _, action := range actions {
		item := actionJSON{ID: action.ID.String(), Name: action.Name}

		if action.NextStepDefinitionID != nil {
			next := action.NextStepDefinitionID.String()
			item.NextStepID = &next
		}

		if action.TerminalWorkflowStatusDefinitionID != nil {
			terminal := action.TerminalWorkflowStatusDefinitionID.String()
			item.TerminalStatusID = &terminal
		}

		payload = append(payload, item)
	}

	return payload
}
