package app

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/mike-akdeniz/flowcore/client/internal/store"
)

// QueueItem is one piece of work waiting on somebody.
//
// Two kinds of thing appear in one list, because "what should I do next" does not
// sort by where the work came from:
//
//   - a **draft**, which nobody has submitted yet, and
//   - an **open step**, which FlowCore is holding for whoever it is assigned to.
//
// Only the second exists in the library. A drafted submission is work CaseWork
// knows about and FlowCore has never heard of, which is why the queue is assembled
// here rather than being a projection of the worklist.
type QueueItem struct {
	Reference    string
	Type         store.SubmissionType
	SubmissionID uuid.UUID
	// StepName is the workflow step waiting, or empty for a draft.
	StepName string
	Assignee string
	// VisitID is what a decision acts on. Nil for a draft.
	VisitID      *uuid.UUID
	WaitingSince time.Time
	IsDraft      bool
	// Finished is set only by AllWork, which is the one list that shows closed
	// cases. A worklist never does: nothing is waiting on anyone.
	Finished bool
}

// Queue is what is waiting on this person: their drafts, and the open steps
// assigned to them or to a group they belong to.
func (a *App) Queue(ctx context.Context, sessionID string, identity Identity) ([]QueueItem, error) {
	submissions, err := a.Store.Submissions(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	items := make([]QueueItem, 0, len(submissions))

	// Drafts sit with intake, whose job is taking details and submitting them.
	if identity.CanActAs("group:intake-handlers") {
		for _, submission := range submissions {
			if !submission.IsDraft() {
				continue
			}

			items = append(items, QueueItem{
				Reference:    submission.Reference,
				Type:         submission.Type,
				SubmissionID: submission.ID,
				Assignee:     "group:intake-handlers",
				WaitingSince: submission.CreatedAt,
				IsDraft:      true,
			})
		}
	}

	assigned, err := a.Worklist(ctx, sessionID, identity)
	if err != nil {
		return nil, err
	}

	byReference := make(map[string]store.Submission, len(submissions))
	for _, submission := range submissions {
		if submission.SubjectReference != nil {
			byReference[*submission.SubjectReference] = submission
		}
	}

	for _, step := range assigned {
		submission, ok := byReference[step.SubjectReference]
		if !ok {
			continue
		}

		visitID := step.VisitID
		items = append(items, QueueItem{
			Reference:    submission.Reference,
			Type:         submission.Type,
			SubmissionID: submission.ID,
			StepName:     step.StepName,
			Assignee:     step.AssigneeID,
			VisitID:      &visitID,
			WaitingSince: step.EnteredAt,
		})
	}

	// One list, newest first. Drafts and open steps are not separated, for the
	// same reason claims and applications are not: "what should I do next" does
	// not sort by where the work came from.
	//
	// Newest rather than oldest, which is the other defensible answer — a real
	// console would likely lead with what has waited longest, because ageing is
	// what an SLA measures. Newest-first suits a demonstration, where the thing
	// you just did should be the thing you see.
	sort.Slice(items, func(i, j int) bool {
		return items[i].WaitingSince.After(items[j].WaitingSince)
	})

	return items, nil
}

// AllWork is every submission this session has, whoever it is waiting on.
//
// The worklist answers "what is waiting on me", which is the question a console
// asks most and the only one FlowCore can help with — it matches assignees and
// has no notion of a case. This answers "where is everything", which needs
// CaseWork's own submission table joined to whatever the library says about each
// run, and which is otherwise unanswerable without switching identity until the
// case you want appears in somebody's queue.
func (a *App) AllWork(ctx context.Context, sessionID string) ([]QueueItem, error) {
	submissions, err := a.Store.Submissions(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	items := make([]QueueItem, 0, len(submissions))

	for _, submission := range submissions {
		item := QueueItem{
			Reference:    submission.Reference,
			Type:         submission.Type,
			SubmissionID: submission.ID,
			WaitingSince: submission.CreatedAt,
			IsDraft:      submission.IsDraft(),
		}

		if submission.IsDraft() {
			item.Assignee = "group:intake-handlers"
			items = append(items, item)

			continue
		}

		state, err := a.Engine.GetState(ctx, *submission.SubjectReference, *submission.FlowcoreDefinitionID)
		if err != nil {
			// A run the library cannot find, which should not happen and is not
			// worth hiding the case over: the submission is real either way.
			items = append(items, item)

			continue
		}

		if state.CurrentStep == nil {
			// Finished. The status is the terminating action's, frozen when it
			// was stamped, so a closed case still says how it ended.
			item.StepName = state.WorkflowStatusName
			item.Finished = true
			items = append(items, item)

			continue
		}

		visitID := state.CurrentStep.VisitID
		item.StepName = state.CurrentStep.Name
		item.Assignee = state.CurrentStep.AssigneeID
		item.VisitID = &visitID
		item.WaitingSince = state.CurrentStep.EnteredAt

		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].WaitingSince.After(items[j].WaitingSince)
	})

	return items, nil
}
