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

// CompleteStep records a decision by this identity.
//
// CompletedBy is the identity's opaque reference. Note what is not checked: the
// library never asks whether this person was the assignee. That is what makes a
// human override of an agent step possible with no special mechanism — the
// completer simply need not be the assignee.
func (a *App) CompleteStep(
	ctx context.Context,
	identity Identity,
	request CompleteRequest,
) (flowcore.WorkflowState, error) {
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
				Label:     member.Name + " — " + member.Title,
				Kind:      KindPerson,
			})

			return
		}

		assignees = append(assignees, Assignee{
			Reference: reference,
			Label:     teamLabel(reference),
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

	for _, workflow := range registered {
		definition, err := a.Catalog.Get(ctx, workflow.FlowcoreDefinitionID)
		if err != nil {
			return nil, err
		}

		for _, step := range definition.Steps {
			add(step.AssigneeID)
		}
	}

	return assignees, nil
}

// teamLabel makes a group reference readable: "group:senior-uw" reads as
// "Senior UW".
//
// Cosmetic only, and deliberately so — a team has no name anywhere in CaseWork,
// because a group exists solely as a string on a person and on a step. Inventing
// display names in a table would be inventing data.
//
// The acronym list will go stale, and that is tolerable here in a way the stale
// roster was not: a missing entry yields "Siu" instead of "SIU", which is ugly.
// It cannot produce a wrong assignment, because the reference travels untouched
// and this touches only what is shown.
func teamLabel(reference string) string {
	acronyms := map[string]string{"siu": "SIU", "uw": "UW", "qa": "QA"}

	_, name, found := strings.Cut(reference, ":")
	if !found {
		name = reference
	}

	words := strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' })
	for i, word := range words {
		if expanded, ok := acronyms[word]; ok {
			words[i] = expanded

			continue
		}

		words[i] = strings.ToUpper(word[:1]) + word[1:]
	}

	return strings.Join(words, " ")
}
