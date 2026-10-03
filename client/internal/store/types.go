package store

import (
	"time"

	"github.com/google/uuid"
)

// Staff is one of the seeded cast. Shared across sessions and read-only: what
// belongs to a visitor is the work, not the people.
type Staff struct {
	Reference string
	Name      string
	// Groups are the references this person also answers to. Expanding a person
	// into their groups is what CaseWork does before asking FlowCore for a
	// worklist, because the library has no identity model to do it with.
	Groups []string
}

// SubmissionType is what arrived. It selects the workflow, the detail table, and
// which screen renders it.
type SubmissionType string

const (
	TypeClaim       SubmissionType = "claim"
	TypeApplication SubmissionType = "application"
)

// Submission is the queue's view of a piece of work, common to both types.
type Submission struct {
	ID          uuid.UUID
	SessionID   string
	Type        SubmissionType
	Reference   string
	Status      string // draft | submitted
	CreatedAt   time.Time
	SubmittedAt *time.Time
	// FlowcoreDefinitionID is the workflow this was submitted under, frozen at
	// submission. Nil while a draft. Never rewritten — a run keeps the workflow it
	// started with, and rewriting this is the one way CaseWork could undermine
	// that.
	FlowcoreDefinitionID *uuid.UUID
	// SubjectReference is what FlowCore was given: opaque to the library, and
	// prefixed with the session so two visitors working the same seeded claim have
	// two separate runs.
	SubjectReference *string
	// Revision is bumped whenever a document is added or removed or a detail edited, and is
	// what CaseWork passes FlowCore as the subject version token. The library
	// records it and never compares it — noticing that a subject moved on is this
	// layer's job.
	Revision int
}

func (s Submission) IsDraft() bool { return s.Status == "draft" }

// ClaimDetail is everything the claim screens show and the claim agents read.
type ClaimDetail struct {
	SubmissionID      uuid.UUID
	PolicyNumber      string
	ClaimantName      string
	Amount            string
	OccurredAt        time.Time
	IncidentNarrative string
}

// ApplicationDetail is the same for a new policy application.
type ApplicationDetail struct {
	SubmissionID uuid.UUID
	ProposerName string
	CoverType    string
	SumInsured   string
	Disclosures  string
}

// Document is a record carrying text, not a file. The documents that matter are
// prose, and the text is what the agents read.
//
// Rows are never overwritten. Unused documents can be deleted while draft.
// A newer document of the same kind supersedes an older one — see Current.
type Document struct {
	ID           uuid.UUID
	SubmissionID uuid.UUID
	Name         string
	// DocumentTypeID is the type's stable id, and what currency is keyed on.
	DocumentTypeID uuid.UUID
	// Kind is the type's name, read with the document for the callers that match
	// on it — the samples. Never written: the id is what a document stores.
	Kind string
	// Title is the type's title, "Police report", read with the document because
	// it is how an agent's instructions refer to it. Never written either.
	Title      string
	ReceivedAt time.Time
	Body       *string
	// SourceFile is the sample file this came from, if it came from one.
	SourceFile *string
	// AddedAtRevision is the submission revision this document arrived at.
	AddedAtRevision int
}

// Current returns the document of each kind in force at a revision: the newest
// one of that kind to have arrived at or before it.
//
// Currency is per type, not per case. That is the whole rule, and it is why a
// step reading several kinds at once — `triage` wants an intake note, an estimate
// and a police report — needs no tie-break: two documents only compete when they
// are the same kind.
//
// Pass the submission's own revision for what an agent should read now, or the
// revision a visit stamped for what that visit read then. The second is what
// keeps a completed decision legible after the file has moved on.
//
// documents must be ordered by AddedAtRevision, which is how Store.Documents
// returns them.
func Current(documents []Document, asOfRevision int) []Document {
	newest := make(map[uuid.UUID]Document, len(documents))
	for _, document := range documents {
		if document.AddedAtRevision <= asOfRevision {
			newest[document.DocumentTypeID] = document
		}
	}

	current := make([]Document, 0, len(newest))
	for _, document := range documents {
		if newest[document.DocumentTypeID].ID == document.ID {
			current = append(current, document)
		}
	}

	return current
}

// RegisteredWorkflow associates a submission type with a FlowCore definition.
//
// The library takes a definition id and starts a run. Which definition a claim
// should use is a question it cannot answer, so CaseWork answers it here.
type RegisteredWorkflow struct {
	ID                   uuid.UUID
	SessionID            string
	SubmissionType       SubmissionType
	Name                 string
	FlowcoreDefinitionID uuid.UUID
	Active               bool
	CreatedAt            time.Time
}

// DocumentType is a kind of document CaseWork knows about.
//
// Configuration rather than a constant: a visitor can add one. Which kinds of
// case may hold it is the allowed list; which steps require it is on the
// workflow definition in FlowCore, as the type's id.
type DocumentType struct {
	ID        uuid.UUID
	SessionID string
	// Name is the middle segment of a sample's file name, and the handle the
	// browser uses. Not editable; Title is.
	Name      string
	Title     string
	CreatedAt time.Time
}

// ReplayStep is one recorded answer a session's seeded agent step replays on its
// seeded case (client decision 69). The ids are the session's own, resolved when
// it was seeded.
type ReplayStep struct {
	SessionID          string
	StepDefinitionID   uuid.UUID
	Reference          string
	ActionDefinitionID uuid.UUID
	Finding            string
}
