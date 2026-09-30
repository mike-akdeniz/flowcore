package app

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/mike-akdeniz/flowcore"
	"github.com/mike-akdeniz/flowcore/client/internal/store"
)

// Document types across the boundary.
//
// CaseWork owns the types: their ids, names, titles, simulated findings, and
// which kinds of case may hold them. FlowCore holds only which of them a step
// requires, as opaque ids on the step definition, frozen into every run at start
// (FlowCore decision 47). The rules here are CaseWork's, enforced over both:
// a step may require only what its kind of case allows, a type may leave that
// list only when nothing could still require it, and an agent may hand work to
// another agent only with the documents that agent needs.

// submissionTypeOf is the kind of case a workflow was registered for. A
// definition is registered under one type, and that is what bounds the document
// types its steps may require.
func (a *App) submissionTypeOf(ctx context.Context, sessionID string, definitionID uuid.UUID) (store.SubmissionType, error) {
	registered, err := a.Store.RegisteredWorkflows(ctx, sessionID)
	if err != nil {
		return "", err
	}

	for _, workflow := range registered {
		if workflow.FlowcoreDefinitionID == definitionID {
			return workflow.SubmissionType, nil
		}
	}

	return "", ErrNotYours
}

// requiredTypeIDs resolves the document type names an editor sent into the ids
// FlowCore stores, refusing any type not allowed on the workflow's kind of case.
//
// The refusal is the whole point. A step requiring a document the case cannot
// hold could never be decided.
func (a *App) requiredTypeIDs(
	ctx context.Context,
	sessionID string,
	definitionID uuid.UUID,
	names []string,
) ([]string, error) {
	submissionType, err := a.submissionTypeOf(ctx, sessionID, definitionID)
	if err != nil {
		return nil, err
	}

	allowed, err := a.Store.AllowedDocumentTypes(ctx, sessionID, submissionType)
	if err != nil {
		return nil, err
	}

	byName := make(map[string]store.DocumentType, len(allowed))
	for _, documentType := range allowed {
		byName[documentType.Name] = documentType
	}

	ids := make([]string, 0, len(names))
	for _, name := range names {
		documentType, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("%q is not a document type a %s may hold, so no step can require it",
				name, submissionType)
		}

		ids = append(ids, documentType.ID.String())
	}

	return ids, nil
}

// checkAgentHandoffs refuses an action from one agent step to another unless the
// destination's required types are among the source's (client decision 38).
//
// After submission only the current assignee may file a document, so nothing
// can be added while an agent holds a step. The source cannot be decided without
// its own required types, so if the destination's are among them they are
// present when it hands over — and if not, the source's decision would be
// blocked with nobody able to unblock it.
//
// Only actions into or out of the edited step are checked. A definition that
// already broke the rule elsewhere should not stop an unrelated edit, and the
// edit that broke it was refused where it happened.
func (a *App) checkAgentHandoffs(
	ctx context.Context,
	sessionID string,
	definition flowcore.WorkflowDefinition,
	editedStepID uuid.UUID,
) error {
	steps := make(map[uuid.UUID]flowcore.StepDefinition, len(definition.Steps))
	for _, step := range definition.Steps {
		steps[step.ID] = step
	}

	for _, source := range definition.Steps {
		if !IsAgent(source.AssigneeID) {
			continue
		}

		for _, action := range source.Actions {
			if action.NextStepDefinitionID == nil {
				continue
			}

			destination, ok := steps[*action.NextStepDefinitionID]
			if !ok || !IsAgent(destination.AssigneeID) {
				continue
			}

			if source.ID != editedStepID && destination.ID != editedStepID {
				continue
			}

			missing := make([]string, 0)
			for _, id := range destination.RequiredInputTypeIDs {
				if !slices.Contains(source.RequiredInputTypeIDs, id) {
					missing = append(missing, id)
				}
			}

			if len(missing) == 0 {
				continue
			}

			titles, err := a.documentTypeTitles(ctx, sessionID, missing)
			if err != nil {
				return err
			}

			return fmt.Errorf(
				"%q hands %q to another agent, which requires %s — require %s on %q as well, "+
					"or put a person between them who can file it",
				source.Name+" / "+action.Name, destination.Name, strings.Join(titles, ", "),
				map[bool]string{true: "it", false: "them"}[len(titles) == 1], source.Name)
		}
	}

	return nil
}

// documentTypeTitles names type ids the way screens do. An id this session does
// not know is shown as it is rather than dropped, so an error never hides what
// it is about.
func (a *App) documentTypeTitles(ctx context.Context, sessionID string, ids []string) ([]string, error) {
	types, err := a.Store.DocumentTypes(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	titles := make(map[string]string, len(types))
	for _, documentType := range types {
		titles[documentType.ID.String()] = documentType.Title
	}

	named := make([]string, 0, len(ids))
	for _, id := range ids {
		if title, ok := titles[id]; ok {
			named = append(named, title)
		} else {
			named = append(named, id)
		}
	}

	return named, nil
}

// SetAllowedDocumentTypes replaces the list of document types a kind of case may
// hold with exactly this set, by name.
//
// Adding is always allowed. Removing a type is refused while anything could
// still require it on a case of this kind (client decision 37):
//   - a workflow registered for this kind of case whose definition requires it,
//     which has to be edited first; or
//   - an open run of any of those workflows whose snapshot requires it on any
//     step, reached or not, because that run will still demand it.
//
// A finished run does not block removal. It keeps its frozen requirements and
// its history still explains them, but it will never ask for the document again.
//
// The checks read and then write, so a concurrent start or edit can slip between
// them. They are the editor's rule, not a lock.
func (a *App) SetAllowedDocumentTypes(
	ctx context.Context,
	sessionID string,
	submissionType store.SubmissionType,
	names []string,
) error {
	known, err := a.Store.DocumentTypes(ctx, sessionID)
	if err != nil {
		return err
	}

	byName := make(map[string]store.DocumentType, len(known))
	for _, documentType := range known {
		byName[documentType.Name] = documentType
	}

	wanted := make(map[uuid.UUID]bool, len(names))
	for _, name := range names {
		documentType, ok := byName[name]
		if !ok {
			return fmt.Errorf("no document type named %q", name)
		}

		wanted[documentType.ID] = true
	}

	current, err := a.Store.AllowedDocumentTypes(ctx, sessionID, submissionType)
	if err != nil {
		return err
	}

	for _, documentType := range current {
		if wanted[documentType.ID] {
			continue
		}

		if err := a.checkRemovable(ctx, sessionID, submissionType, documentType); err != nil {
			return err
		}
	}

	for _, documentType := range current {
		if !wanted[documentType.ID] {
			if err := a.Store.DisallowDocumentType(ctx, documentType.ID, submissionType); err != nil {
				return err
			}
		}
	}

	for id := range wanted {
		if err := a.Store.AllowDocumentType(ctx, id, submissionType); err != nil {
			return err
		}
	}

	return nil
}

// checkRemovable is the guard on taking a type off a kind of case's allowed
// list: no registered definition for it may require the type, and no open run of
// one may have it in its snapshot.
func (a *App) checkRemovable(
	ctx context.Context,
	sessionID string,
	submissionType store.SubmissionType,
	documentType store.DocumentType,
) error {
	registered, err := a.Store.RegisteredWorkflows(ctx, sessionID)
	if err != nil {
		return err
	}

	id := documentType.ID.String()
	definitionIDs := make([]uuid.UUID, 0, len(registered))

	for _, workflow := range registered {
		if workflow.SubmissionType != submissionType {
			continue
		}

		definitionIDs = append(definitionIDs, workflow.FlowcoreDefinitionID)

		definition, err := a.Catalog.Get(ctx, workflow.FlowcoreDefinitionID)
		if err != nil {
			return err
		}

		for _, step := range definition.Steps {
			if slices.Contains(step.RequiredInputTypeIDs, id) {
				return fmt.Errorf("%s is required by %q in %q, so a %s must still be able to hold it — "+
					"remove the requirement first",
					documentType.Title, step.Name, definition.Name, submissionType)
			}
		}
	}

	// From the runs themselves, not the definitions: a run started before the
	// requirement was removed still demands the document on steps it has not
	// reached yet.
	steps, err := a.Engine.ListOpenRunSteps(ctx, definitionIDs)
	if err != nil {
		return err
	}

	for _, step := range steps {
		if slices.Contains(step.RequiredInputTypeIDs, id) {
			return fmt.Errorf("%s is still required by %q on a %s in progress (%s), so it cannot be removed "+
				"until that case has finished",
				documentType.Title, step.Name, submissionType, referenceOf(subjectOf(step.SubjectReference)))
		}
	}

	return nil
}

// RetitleDocumentType changes what a type is called on screen. Its id is what
// documents, allowed lists and every run's requirements refer to, so none of
// them change, and a finished case's history shows the new title for the same
// type.
func (a *App) RetitleDocumentType(ctx context.Context, sessionID, name, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("a document type needs a title")
	}

	documentType, err := a.DocumentTypeNamed(ctx, sessionID, name)
	if err != nil {
		return err
	}

	return a.Store.RetitleDocumentType(ctx, sessionID, documentType.ID, title)
}

// AllowedDocumentTypeNames is what may be filed on a kind of case, by name, for
// the Add document selector — the whole list, at every step and in draft.
func (a *App) AllowedDocumentTypeNames(
	ctx context.Context,
	sessionID string,
	submissionType store.SubmissionType,
) ([]string, error) {
	allowed, err := a.Store.AllowedDocumentTypes(ctx, sessionID, submissionType)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(allowed))
	for _, documentType := range allowed {
		names = append(names, documentType.Name)
	}

	return names, nil
}

// AllowedDocumentType resolves a name to a type this kind of case may hold, or
// explains why it cannot be filed. Every way a document arrives — upload, sample,
// seed — goes through the same list.
func (a *App) AllowedDocumentType(
	ctx context.Context,
	sessionID string,
	submissionType store.SubmissionType,
	name string,
) (store.DocumentType, error) {
	allowed, err := a.Store.AllowedDocumentTypes(ctx, sessionID, submissionType)
	if err != nil {
		return store.DocumentType{}, err
	}

	for _, documentType := range allowed {
		if documentType.Name == name {
			return documentType, nil
		}
	}

	return store.DocumentType{}, fmt.Errorf("a %s cannot hold a document of type %q", submissionType, name)
}
