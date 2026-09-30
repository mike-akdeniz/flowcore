package app

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/mike-akdeniz/flowcore"
	"github.com/mike-akdeniz/flowcore/client/internal/store"
)

// Worklist returns the open steps waiting on this identity, within this session.
//
// Two things happen here that FlowCore deliberately will not do. The identity is
// expanded into the set of references it answers to — itself plus its groups —
// because deciding who belongs to a group needs an identity model the library
// does not have. And the result is filtered to this session's own definitions,
// because the library has no tenant and the worklist spans every run in the
// database.
//
// The filter is possible because AssignedStep carries WorkflowDefinitionID, so no
// assignee string has to be namespaced: a visitor who types "group:security" into
// the configuration form gets exactly that stored.
func (a *App) Worklist(ctx context.Context, sessionID string, identity Identity) ([]flowcore.AssignedStep, error) {
	assigned, err := a.Engine.ListAssignedSteps(ctx, identity.WorklistReferences())
	if err != nil {
		return nil, err
	}

	mine := make([]flowcore.AssignedStep, 0, len(assigned))
	for _, step := range assigned {
		owned, err := a.owns(ctx, sessionID, step.WorkflowDefinitionID)
		if err != nil {
			return nil, err
		}

		if owned {
			mine = append(mine, step)
		}
	}

	return mine, nil
}

// CompleteRequest is what the client needs to record a decision.
type CompleteRequest struct {
	VisitID  uuid.UUID
	ActionID uuid.UUID
	// Remark is optional: why this decision was made, stamped in the same
	// transaction as the decision itself so a crash cannot separate them.
	Remark string
	// SubjectVersionToken is the revision the decision was made against. The
	// client supplies it from its own store; FlowCore records it and never
	// compares it, so noticing that a subject moved on is this layer's job.
	SubjectVersionToken string
}

// CompleteStep records a decision by this identity on the step a case is
// waiting on, once its required documents are there (see checkDecision).
//
// step is the open step as the caller read it. If the request names a different
// visit the caller's view is stale, and it is refused the way FlowCore would
// refuse it, rather than checked against a step it is not deciding.
//
// CompletedBy is the identity's opaque reference. Note what is not checked: the
// library never asks whether this person was the assignee. That is what makes a
// human override of an agent step possible with no special mechanism — the
// completer simply need not be the assignee.
func (a *App) CompleteStep(
	ctx context.Context,
	sessionID string,
	submission store.Submission,
	step flowcore.CurrentStep,
	identity Identity,
	request CompleteRequest,
) (flowcore.WorkflowState, error) {
	if request.VisitID != step.VisitID {
		return flowcore.WorkflowState{}, &flowcore.VisitNotOpenError{VisitID: request.VisitID}
	}

	if err := a.checkDecision(ctx, sessionID, submission, step, request.ActionID); err != nil {
		return flowcore.WorkflowState{}, err
	}

	params := flowcore.CompleteParams{
		VisitID:     request.VisitID,
		ActionID:    request.ActionID,
		CompletedBy: identity.Reference,
	}

	if request.Remark != "" {
		params.Remark = &request.Remark
	}

	if request.SubjectVersionToken != "" {
		params.SubjectVersionToken = &request.SubjectVersionToken
	}

	return a.Engine.CompleteStep(ctx, params)
}

// Reassign moves an open visit to another assignee.
//
// There is no unassign: FlowCore requires a value, and work with no assignee
// would match no worklist query, so releasing it that way would hide it rather
// than free it.
func (a *App) Reassign(ctx context.Context, visitID uuid.UUID, assignee string) (flowcore.WorkflowState, error) {
	return a.Engine.Reassign(ctx, visitID, assignee)
}

// Assignee is somebody a step can be handed to, with a label for the interface.
//
// Kind is what lets the interface group them. A flat list of references — the
// first version of this — reads as a wall of prefixed strings in which a person,
// a team and a machine are indistinguishable.
type Assignee struct {
	Reference string
	Label     string
	// Kind is "person" or "team".
	Kind string
}

const (
	KindPerson = "person"
	KindTeam   = "team"
)

// AssignableReferences is everyone a step can be handed to: each member of the
// cast, and each team named either by their memberships or by a workflow this
// session has registered.
//
// All of it comes from the database, and that is the point of the signature. An
// earlier version read two Go literals instead, and drifted: it offered a cast
// from a scenario the client had already replaced, so the list named people who
// did not exist while omitting every group the live workflows use. Nothing
// failed, because FlowCore accepts any string as an assignee — the list was
// simply wrong, quietly, for as long as it took someone to open the dropdown.
//
// Session-scoped because workflows are. A visitor's own definitions are the only
// ones whose assignees can appear.
//
// **Agents are excluded**, though the library would accept one perfectly well.
// Handing work back to a machine is possible and nothing needs it: the recovery
// path runs the other way, agent to human. It was the one entry in the list that
// earned nothing and confused everybody.
func (a *App) AssignableReferences(ctx context.Context, sessionID string) ([]Assignee, error) {
	cast, err := a.Store.Roster(ctx)
	if err != nil {
		return nil, err
	}

	people := make(map[string]store.Staff, len(cast))
	for _, member := range cast {
		people[member.Reference] = member
	}

	seen := make(map[string]bool)

	var assignees []Assignee

	add := func(reference string) {
		if reference == "" || seen[reference] || IsAgent(reference) {
			return
		}

		seen[reference] = true

		// The staff table decides who is a person. Anything else that is not an
		// agent is a team — which means no parsing of the "user:" prefix, and a
		// reference nobody recognises still lands somewhere sensible.
		if member, ok := people[reference]; ok {
			assignees = append(assignees, Assignee{
				Reference: reference,
				Label:     member.Name + " — " + TeamsOf(member.Groups),
				Kind:      KindPerson,
			})

			return
		}

		assignees = append(assignees, Assignee{
			Reference: reference,
			Label:     TeamLabel(reference),
			Kind:      KindTeam,
		})
	}

	for _, member := range cast {
		add(member.Reference)

		for _, group := range member.Groups {
			add(group)
		}
	}

	registered, err := a.Store.RegisteredWorkflows(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	definitionIDs := make([]uuid.UUID, 0, len(registered))
	for _, workflow := range registered {
		definitionIDs = append(definitionIDs, workflow.FlowcoreDefinitionID)

		definition, err := a.Catalog.Get(ctx, workflow.FlowcoreDefinitionID)
		if err != nil {
			return nil, err
		}

		for _, step := range definition.Steps {
			add(step.AssigneeID)
		}
	}

	// And every assignee a running case can still meet, from the runs themselves:
	// a team a definition has since stopped using is still who a run started
	// before the edit will ask for, and has to stay reassignable to.
	steps, err := a.Engine.ListOpenRunSteps(ctx, definitionIDs)
	if err != nil {
		return nil, err
	}

	for _, step := range steps {
		add(step.AssigneeID)
	}

	return assignees, nil
}

// TeamLabel makes a group reference readable: "group:claims-adjusters" reads as
// "Claims adjusters".
//
// Cosmetic only, and deliberately so — a team has no name anywhere in CaseWork,
// because a group exists solely as a string on a person and on a step. Inventing
// display names in a table would be inventing data.
//
// There was an acronym map here, for `group:siu` and `group:senior-uw`. The
// groups were renamed to read as their own labels instead, which deleted it: a
// reference that spells itself needs no translation, and a translation table is
// one more thing that can drift from what it describes.
func TeamLabel(reference string) string {
	_, name, found := strings.Cut(reference, ":")
	if !found {
		name = reference
	}

	// Only the first word is capitalised: "Claims adjusters", not "Claims
	// Adjusters". A team is a noun phrase, not a title.
	words := strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' })
	if len(words) == 0 {
		return reference
	}

	words[0] = strings.ToUpper(words[0][:1]) + words[0][1:]

	return strings.Join(words, " ")
}

// TeamsOf spells a person's groups for display.
//
// This replaced a job title on `staff`, which said the same thing in different
// words — "Claims adjuster" beside a group reading "Adjusters" — and said it
// about the half that does no work. The group is what gets matched against a
// step's assignee, so it is the half that explains why somebody's queue looks the
// way it does.
func TeamsOf(groups []string) string {
	labels := make([]string, 0, len(groups))
	for _, group := range groups {
		labels = append(labels, TeamLabel(group))
	}

	return strings.Join(labels, " · ")
}
