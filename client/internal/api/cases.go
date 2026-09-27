package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/mike-akdeniz/flowcore"
	"github.com/mike-akdeniz/flowcore/client/internal/app"
	"github.com/mike-akdeniz/flowcore/client/internal/store"
)

// One request per screen, returning a case already composed — the submission, its
// type-specific detail, its documents, and where its run stands. React renders
// this; it does not assemble it from library resources.

type documentJSON struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Kind       string  `json:"kind"`
	ReceivedAt string  `json:"receivedAt"`
	Body       *string `json:"body"`
	SourceFile *string `json:"sourceFile"`
	// AddedAtRevision is when this document arrived, and Superseded says a newer
	// one of its kind has taken over. Superseded documents are sent rather than
	// filtered out: an agent's remark refers to the document it actually read,
	// and hiding that document would leave the remark looking wrong.
	AddedAtRevision int  `json:"addedAtRevision"`
	Superseded      bool `json:"superseded"`
}

// visitJSON is one entry into a step, with what was decided and what the file
// looked like when it was.
//
// DocumentIDs is the point of carrying the revision at all. A step reached twice
// by the `awaiting documents` loop leaves two visits with the same name and
// different answers, and this is what shows why: the documents in force at the
// revision each one stamped. Derived here rather than in the browser, because
// `store.Current` is the rule and there should be one of it.
type visitJSON struct {
	StepName    string  `json:"stepName"`
	Assignee    string  `json:"assignee"`
	IsAgent     bool    `json:"isAgent"`
	EnteredAt   string  `json:"enteredAt"`
	CompletedAt *string `json:"completedAt"`
	CompletedBy *string `json:"completedBy"`
	ActionName  *string `json:"actionName"`
	Remark      *string `json:"remark"`
	// Revision is the subject version token, which CaseWork writes as its own
	// revision number. FlowCore stores the string and never reads it, so parsing
	// it back is this layer's business and nobody else's.
	Revision    *int     `json:"revision"`
	DocumentIDs []string `json:"documentIds"`
}

type currentStepJSON struct {
	Name         string       `json:"name"`
	Assignee     string       `json:"assignee"`
	IsAgent      bool         `json:"isAgent"`
	WaitingSince string       `json:"waitingSince"`
	VisitID      string       `json:"visitId"`
	Actions      []actionJSON `json:"actions"`
}

type caseJSON struct {
	Reference    string  `json:"reference"`
	Type         string  `json:"type"`
	Status       string  `json:"status"`
	WorkflowName string  `json:"workflowName"`
	RunStatus    string  `json:"runStatus"`
	SubmittedAt  *string `json:"submittedAt"`
	// Revision is what the current documents are current as of, and what the next
	// completion will stamp on its visit.
	Revision int `json:"revision"`

	Claim       *claimJSON       `json:"claim"`
	Application *applicationJSON `json:"application"`
	Documents   []documentJSON   `json:"documents"`
	CurrentStep *currentStepJSON `json:"currentStep"`
	// History is every visit, oldest first, and empty on a draft. It rides on the
	// case rather than having an endpoint of its own because the screen polls the
	// case while an agent holds the step — a separate endpoint would mean polling
	// twice, or a history that lags the step it explains.
	History []visitJSON `json:"history"`
}

type claimJSON struct {
	PolicyNumber      string `json:"policyNumber"`
	ClaimantName      string `json:"claimantName"`
	Amount            string `json:"amount"`
	OccurredAt        string `json:"occurredAt"`
	IncidentNarrative string `json:"incidentNarrative"`
}

type applicationJSON struct {
	ProposerName string `json:"proposerName"`
	CoverType    string `json:"coverType"`
	SumInsured   string `json:"sumInsured"`
	Disclosures  string `json:"disclosures"`
}

func (s *Server) showCase(w http.ResponseWriter, r *http.Request) {
	sessionID := sessionFrom(r)

	submission, err := s.app.Store.SubmissionByReference(r.Context(), sessionID, r.PathValue("reference"))
	if err != nil {
		http.Error(w, "no such case", http.StatusNotFound)

		return
	}

	payload, err := s.composeCase(r, sessionID, submission)
	if err != nil {
		s.fail(w, "could not read the case", err)

		return
	}

	s.write(w, payload)
}

func (s *Server) composeCase(r *http.Request, sessionID string, submission store.Submission) (caseJSON, error) {
	payload := caseJSON{
		Reference: submission.Reference,
		Type:      string(submission.Type),
		Status:    submission.Status,
		Revision:  submission.Revision,
		History:   []visitJSON{},
	}

	if submission.SubmittedAt != nil {
		at := submission.SubmittedAt.Format(time.RFC3339)
		payload.SubmittedAt = &at
	}

	switch submission.Type {
	case store.TypeClaim:
		detail, err := s.app.Store.ClaimDetail(r.Context(), submission.ID)
		if err != nil {
			return caseJSON{}, err
		}

		payload.Claim = &claimJSON{
			PolicyNumber:      detail.PolicyNumber,
			ClaimantName:      detail.ClaimantName,
			Amount:            detail.Amount,
			OccurredAt:        detail.OccurredAt.Format("2006-01-02"),
			IncidentNarrative: detail.IncidentNarrative,
		}
	case store.TypeApplication:
		detail, err := s.app.Store.ApplicationDetail(r.Context(), submission.ID)
		if err != nil {
			return caseJSON{}, err
		}

		payload.Application = &applicationJSON{
			ProposerName: detail.ProposerName,
			CoverType:    detail.CoverType,
			SumInsured:   detail.SumInsured,
			Disclosures:  detail.Disclosures,
		}
	}

	documents, err := s.app.Store.Documents(r.Context(), submission.ID)
	if err != nil {
		return caseJSON{}, err
	}

	// Superseded is derived, not stored: a document is superseded exactly when a
	// newer one of its kind exists, so there is no flag that can drift out of step
	// with the rows it describes.
	inForce := make(map[uuid.UUID]bool)
	for _, document := range store.Current(documents, submission.Revision) {
		inForce[document.ID] = true
	}

	payload.Documents = make([]documentJSON, 0, len(documents))
	for _, document := range documents {
		payload.Documents = append(payload.Documents, documentJSON{
			ID:              document.ID.String(),
			Name:            document.Name,
			Kind:            document.Kind,
			ReceivedAt:      document.ReceivedAt.Format("2006-01-02"),
			Body:            document.Body,
			SourceFile:      document.SourceFile,
			AddedAtRevision: document.AddedAtRevision,
			Superseded:      !inForce[document.ID],
		})
	}

	// A draft has no run. Everything below only exists once it has been submitted.
	if submission.IsDraft() || submission.SubjectReference == nil {
		return payload, nil
	}

	state, err := s.app.Engine.GetState(r.Context(), *submission.SubjectReference, *submission.FlowcoreDefinitionID)
	if err != nil {
		return caseJSON{}, err
	}

	payload.RunStatus = state.WorkflowStatusName

	registered, err := s.app.Store.RegisteredWorkflows(r.Context(), sessionID)
	if err != nil {
		return caseJSON{}, err
	}

	for _, workflow := range registered {
		if workflow.FlowcoreDefinitionID == *submission.FlowcoreDefinitionID {
			payload.WorkflowName = workflow.Name
		}
	}

	history, err := s.app.Engine.GetHistory(r.Context(),
		*submission.SubjectReference, *submission.FlowcoreDefinitionID)
	if err != nil {
		return caseJSON{}, err
	}

	for _, visit := range history {
		payload.History = append(payload.History, toVisitJSON(visit, documents))
	}

	if state.CurrentStep != nil {
		payload.CurrentStep = &currentStepJSON{
			Name:     state.CurrentStep.Name,
			Assignee: state.CurrentStep.AssigneeID,
			IsAgent:  app.IsAgent(state.CurrentStep.AssigneeID),
			// The visit id passes through the browser untouched. React does not
			// know what a visit is; returning it unchanged is what preserves
			// FlowCore's stale-view protection without the front end understanding
			// why it exists.
			VisitID:      state.CurrentStep.VisitID.String(),
			WaitingSince: state.CurrentStep.EnteredAt.Format(time.RFC3339),
			Actions:      make([]actionJSON, 0, len(state.CurrentStep.Actions)),
		}

		for _, action := range state.CurrentStep.Actions {
			payload.CurrentStep.Actions = append(payload.CurrentStep.Actions,
				actionJSON{ID: action.ID.String(), Name: action.Name})
		}
	}

	return payload, nil
}

func toVisitJSON(visit flowcore.StepVisit, documents []store.Document) visitJSON {
	entry := visitJSON{
		StepName:    visit.StepName,
		Assignee:    visit.AssigneeID,
		IsAgent:     app.IsAgent(visit.AssigneeID),
		EnteredAt:   visit.EnteredAt.Format(time.RFC3339),
		DocumentIDs: []string{},
	}

	if visit.Completion == nil {
		return entry
	}

	at := visit.Completion.At.Format(time.RFC3339)
	entry.CompletedAt = &at
	entry.CompletedBy = &visit.Completion.By
	entry.ActionName = &visit.Completion.ActionName
	entry.Remark = visit.Completion.Remark

	// An unparseable token is not an error. FlowCore accepts any string, so a run
	// started by something other than CaseWork — or by an older CaseWork — would
	// carry one this cannot read, and the right answer is to show the decision
	// without claiming to know what it was made against.
	if visit.Completion.SubjectVersionToken == nil {
		return entry
	}

	revision, err := strconv.Atoi(*visit.Completion.SubjectVersionToken)
	if err != nil {
		return entry
	}

	entry.Revision = &revision
	for _, document := range store.Current(documents, revision) {
		entry.DocumentIDs = append(entry.DocumentIDs, document.ID.String())
	}

	return entry
}

// --- creating and submitting ----------------------------------------------

type newCaseJSON struct {
	Type string `json:"type"`

	PolicyNumber      string `json:"policyNumber"`
	ClaimantName      string `json:"claimantName"`
	Amount            string `json:"amount"`
	OccurredAt        string `json:"occurredAt"`
	IncidentNarrative string `json:"incidentNarrative"`

	ProposerName string `json:"proposerName"`
	CoverType    string `json:"coverType"`
	SumInsured   string `json:"sumInsured"`
	Disclosures  string `json:"disclosures"`
}

func (s *Server) createCase(w http.ResponseWriter, r *http.Request) {
	var body newCaseJSON
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)

		return
	}

	reference, err := s.app.CreateSubmission(r.Context(), sessionFrom(r), app.NewSubmission{
		Type:              store.SubmissionType(body.Type),
		PolicyNumber:      body.PolicyNumber,
		ClaimantName:      body.ClaimantName,
		Amount:            body.Amount,
		OccurredAt:        body.OccurredAt,
		IncidentNarrative: body.IncidentNarrative,
		ProposerName:      body.ProposerName,
		CoverType:         body.CoverType,
		SumInsured:        body.SumInsured,
		Disclosures:       body.Disclosures,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	s.write(w, map[string]string{"reference": reference})
}

// submitCase is where a draft becomes a run.
//
// Two things happen that FlowCore cannot do for itself: CaseWork decides which
// workflow applies, from the submission's type, and it supplies the opaque
// subject reference the run will be known by.
func (s *Server) submitCase(w http.ResponseWriter, r *http.Request) {
	sessionID := sessionFrom(r)

	submission, err := s.app.Store.SubmissionByReference(r.Context(), sessionID, r.PathValue("reference"))
	if err != nil {
		http.Error(w, "no such case", http.StatusNotFound)

		return
	}

	if err := s.app.Submit(r.Context(), sessionID, submission); err != nil {
		http.Error(w, app.ErrorMessage(err), http.StatusBadRequest)

		return
	}

	updated, err := s.app.Store.SubmissionByReference(r.Context(), sessionID, submission.Reference)
	if err != nil {
		s.fail(w, "could not re-read the case", err)

		return
	}

	payload, err := s.composeCase(r, sessionID, updated)
	if err != nil {
		s.fail(w, "could not read the case", err)

		return
	}

	s.write(w, payload)
}

// --- documents --------------------------------------------------------------

type newDocumentJSON struct {
	// Exactly one of these: a sample chosen from the embedded set, or a file the
	// visitor uploaded, whose text arrives as a string because documents are text
	// records and nothing binary is stored.
	SampleFile string `json:"sampleFile"`
	FileName   string `json:"fileName"`
	Body       string `json:"body"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
}

func (s *Server) addDocument(w http.ResponseWriter, r *http.Request) {
	sessionID := sessionFrom(r)

	submission, err := s.app.Store.SubmissionByReference(r.Context(), sessionID, r.PathValue("reference"))
	if err != nil {
		http.Error(w, "no such case", http.StatusNotFound)

		return
	}

	var body newDocumentJSON
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)

		return
	}

	document := store.Document{
		ID:           uuid.Must(uuid.NewV7()),
		SubmissionID: submission.ID,
		ReceivedAt:   time.Now(),
	}

	if body.SampleFile != "" {
		sample, ok := s.app.Samples.ByName(body.SampleFile)
		if !ok {
			http.Error(w, "no such sample document", http.StatusBadRequest)

			return
		}

		text, fileName := sample.Body, sample.FileName
		document.Name = sample.Title
		document.Kind = sample.Kind
		document.Body = &text
		document.SourceFile = &fileName
	} else {
		if body.Body == "" || body.FileName == "" {
			http.Error(w, "a document needs a file and its text", http.StatusBadRequest)

			return
		}

		text, fileName := body.Body, body.FileName
		document.Name = body.Name
		if document.Name == "" {
			document.Name = body.FileName
		}

		document.Kind = body.Kind
		if document.Kind == "" {
			document.Kind = "correspondence"
		}

		document.Body = &text
		document.SourceFile = &fileName
	}

	if _, err := s.app.Store.AddDocument(r.Context(), document); err != nil {
		s.fail(w, "could not add the document", err)

		return
	}

	// Re-read: adding a document moved the submission's revision, and the payload
	// below reports what is current as of it.
	submission, err = s.app.Store.SubmissionByReference(r.Context(), sessionID, submission.Reference)
	if err != nil {
		s.fail(w, "could not read the case", err)

		return
	}

	payload, err := s.composeCase(r, sessionID, submission)
	if err != nil {
		s.fail(w, "could not read the case", err)

		return
	}

	s.write(w, payload)
}

// --- samples ----------------------------------------------------------------

type sampleJSON struct {
	FileName string `json:"fileName"`
	Title    string `json:"title"`
	Kind     string `json:"kind"`
	Outcome  string `json:"outcome"`
	Body     string `json:"body"`
}

// listSamples serves the embedded set, so a hosted visitor has the same documents
// available as someone who cloned the repository.
func (s *Server) listSamples(w http.ResponseWriter, r *http.Request) {
	documents := s.app.Samples.All()

	payload := make([]sampleJSON, 0, len(documents))
	for _, document := range documents {
		payload = append(payload, sampleJSON{
			FileName: document.FileName,
			Title:    document.Title,
			Kind:     document.Kind,
			Outcome:  string(document.Outcome),
			Body:     document.Body,
		})
	}

	s.write(w, payload)
}

// --- deciding and reassigning ----------------------------------------------

type decideJSON struct {
	VisitID  string `json:"visitId"`
	ActionID string `json:"actionId"`
	Remark   string `json:"remark"`
}

// decide records a human decision on the open step.
//
// The assignee check here is CaseWork's, and it is the whole of the policy —
// FlowCore records who completed a step and never asks whether they were
// allowed to, which is what leaves this to the caller. Client decision 23.
//
// It is enforced here rather than only in the interface because a rule that
// lives in the browser is a suggestion.
func (s *Server) decide(w http.ResponseWriter, r *http.Request) {
	member, ok := s.signedIn(r)
	if !ok {
		http.Error(w, "not signed in", http.StatusUnauthorized)

		return
	}

	sessionID := sessionFrom(r)

	submission, state, ok := s.openStep(w, r, sessionID)
	if !ok {
		return
	}

	var body decideJSON
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)

		return
	}

	identity := app.Identity{
		Reference: member.Reference,
		Name:      member.Name,
		Title:     member.Title,
		Groups:    member.Groups,
	}

	if !identity.CanActAs(state.CurrentStep.AssigneeID) {
		http.Error(w,
			"this step is waiting on "+state.CurrentStep.AssigneeID+
				", so it is not yours to decide — reassign it first",
			http.StatusForbidden)

		return
	}

	visitID, actionID, err := twoIDs(body.VisitID, body.ActionID)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)

		return
	}

	next, err := s.app.CompleteStep(r.Context(), identity, app.CompleteRequest{
		VisitID:  visitID,
		ActionID: actionID,
		Remark:   body.Remark,
		// The same revision an agent would stamp, so both kinds of decision are
		// answerable on the same terms: this is the state of the file the person
		// was looking at.
		SubjectVersionToken: strconv.Itoa(submission.Revision),
	})
	if err != nil {
		s.fail(w, "could not record the decision", err)

		return
	}

	// A human decision can hand the run straight to an agent, and this response
	// already knows whether it did. That is decision 4's whole point — nothing
	// polls to find out — and leaving it off would work, silently, by falling
	// back on the fifteen-second recovery sweep.
	s.app.Dispatcher.Dispatch(sessionID, *submission.FlowcoreDefinitionID, next)

	s.writeCase(w, r, sessionID, submission.Reference)
}

type reassignJSON struct {
	VisitID  string `json:"visitId"`
	Assignee string `json:"assignee"`
}

// reassign moves the open step to somebody else.
//
// Deliberately not restricted to the assignee, unlike deciding. Reassigning
// settles nothing about the claim — the case sits exactly where it sat, and only
// the name beside it changes — and it is what makes a failed agent step
// recoverable, since no person is ever the assignee of one.
func (s *Server) reassign(w http.ResponseWriter, r *http.Request) {
	sessionID := sessionFrom(r)

	submission, _, ok := s.openStep(w, r, sessionID)
	if !ok {
		return
	}

	var body reassignJSON
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Assignee == "" {
		http.Error(w, "bad request", http.StatusBadRequest)

		return
	}

	visitID, err := uuid.Parse(body.VisitID)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)

		return
	}

	next, err := s.app.Reassign(r.Context(), visitID, body.Assignee)
	if err != nil {
		s.fail(w, "could not reassign it", err)

		return
	}

	// Handing a step to an agent is a legitimate move — it is how a person gives
	// work back to one — so this dispatches for the same reason deciding does.
	s.app.Dispatcher.Dispatch(sessionID, *submission.FlowcoreDefinitionID, next)

	s.writeCase(w, r, sessionID, submission.Reference)
}

type assigneeJSON struct {
	Reference string `json:"reference"`
	Label     string `json:"label"`
	// Kind is "person" or "team", so the interface can group them rather than
	// showing one flat list in which they are indistinguishable.
	Kind string `json:"kind"`
}

// assignableReferences is everyone a step can be handed to, for this session.
func (s *Server) assignableReferences(w http.ResponseWriter, r *http.Request) {
	assignees, err := s.app.AssignableReferences(r.Context(), sessionFrom(r))
	if err != nil {
		s.fail(w, "could not list the assignees", err)

		return
	}

	payload := make([]assigneeJSON, 0, len(assignees))
	for _, assignee := range assignees {
		payload = append(payload, assigneeJSON{
			Reference: assignee.Reference,
			Label:     assignee.Label,
			Kind:      assignee.Kind,
		})
	}

	s.write(w, payload)
}

// openStep resolves the case named in the path and its open step, writing the
// error itself if either is missing. Both handlers above need exactly this, and
// both must refuse a finished run rather than acting on a stale visit id.
func (s *Server) openStep(
	w http.ResponseWriter,
	r *http.Request,
	sessionID string,
) (store.Submission, flowcore.WorkflowState, bool) {
	submission, err := s.app.Store.SubmissionByReference(r.Context(), sessionID, r.PathValue("reference"))
	if err != nil {
		http.Error(w, "no such case", http.StatusNotFound)

		return store.Submission{}, flowcore.WorkflowState{}, false
	}

	if submission.IsDraft() || submission.SubjectReference == nil {
		http.Error(w, "this case has not been submitted", http.StatusConflict)

		return store.Submission{}, flowcore.WorkflowState{}, false
	}

	state, err := s.app.Engine.GetState(r.Context(),
		*submission.SubjectReference, *submission.FlowcoreDefinitionID)
	if err != nil {
		s.fail(w, "could not read the case", err)

		return store.Submission{}, flowcore.WorkflowState{}, false
	}

	if state.CurrentStep == nil {
		http.Error(w, "this case has finished", http.StatusConflict)

		return store.Submission{}, flowcore.WorkflowState{}, false
	}

	return submission, state, true
}

func twoIDs(first, second string) (uuid.UUID, uuid.UUID, error) {
	left, err := uuid.Parse(first)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	right, err := uuid.Parse(second)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	return left, right, nil
}

// writeCase re-reads and returns the whole case, which is what every mutating
// handler answers with. The caller then never has to work out what changed —
// deciding can move the run, which changes the step, the history and whether an
// agent now holds it.
func (s *Server) writeCase(w http.ResponseWriter, r *http.Request, sessionID, reference string) {
	submission, err := s.app.Store.SubmissionByReference(r.Context(), sessionID, reference)
	if err != nil {
		s.fail(w, "could not read the case", err)

		return
	}

	payload, err := s.composeCase(r, sessionID, submission)
	if err != nil {
		s.fail(w, "could not read the case", err)

		return
	}

	s.write(w, payload)
}
