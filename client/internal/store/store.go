package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a row a caller named does not exist.
var ErrNotFound = errors.New("casework: not found")

// Store is CaseWork's data access. Hand-written SQL over pgx, matching the
// library's approach in the same repository.
type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Pool exposes the connection for the callers that need the library's own
// constructors, which take a pool of their own.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// --- sessions -------------------------------------------------------------

// TouchSession records a visit, creating the session if it is new. The bool
// reports whether it was created, which is what triggers seeding.
func (s *Store) TouchSession(ctx context.Context, id string) (bool, error) {
	now := time.Now()

	tag, err := s.pool.Exec(ctx,
		`insert into casework.session (id, created_at, last_seen_at)
		 values ($1, $2, $2)
		 on conflict (id) do update set last_seen_at = $2`,
		id, now)
	if err != nil {
		return false, err
	}

	// An insert reports one row; a conflicting update also reports one, so the
	// two are told apart by whether the row existed a moment ago.
	var created bool
	err = s.pool.QueryRow(ctx,
		`select created_at = last_seen_at from casework.session where id = $1`, id).Scan(&created)

	_ = tag

	return created, err
}

// ExpiredSessions returns sessions idle longer than ttl and forgets them. A zero
// ttl returns nothing, which is how "never expire" is expressed.
func (s *Store) ExpiredSessions(ctx context.Context, ttl time.Duration) ([]string, error) {
	if ttl == 0 {
		return nil, nil
	}

	rows, err := s.pool.Query(ctx,
		`delete from casework.session where last_seen_at < $1 returning id`,
		time.Now().Add(-ttl))
	if err != nil {
		return nil, err
	}

	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// --- staff ----------------------------------------------------------------

func (s *Store) Roster(ctx context.Context) ([]Staff, error) {
	rows, err := s.pool.Query(ctx,
		`select reference, name, groups from casework.staff order by sort_order`)
	if err != nil {
		return nil, err
	}

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Staff, error) {
		var member Staff
		err := row.Scan(&member.Reference, &member.Name, &member.Groups)

		return member, err
	})
}

func (s *Store) StaffByReference(ctx context.Context, reference string) (Staff, error) {
	var member Staff
	err := s.pool.QueryRow(ctx,
		`select reference, name, groups from casework.staff where reference = $1`,
		reference).Scan(&member.Reference, &member.Name, &member.Groups)
	if errors.Is(err, pgx.ErrNoRows) {
		return Staff{}, ErrNotFound
	}

	return member, err
}

// --- submissions ----------------------------------------------------------

const submissionColumns = `id, session_id, type, reference, status, created_at,
	submitted_at, flowcore_definition_id, subject_reference, revision`

func scanSubmission(row pgx.CollectableRow) (Submission, error) {
	var submission Submission
	err := row.Scan(&submission.ID, &submission.SessionID, &submission.Type,
		&submission.Reference, &submission.Status, &submission.CreatedAt,
		&submission.SubmittedAt, &submission.FlowcoreDefinitionID, &submission.SubjectReference,
		&submission.Revision)

	return submission, err
}

// InsertSubmission records a new submission. Revision is not among the columns:
// a submission starts at 1 and only AddDocument moves it, so the starting value
// is the schema's default and no caller can get it wrong.
func (s *Store) InsertSubmission(ctx context.Context, submission Submission) error {
	_, err := s.pool.Exec(ctx,
		`insert into casework.submission
		 (id, session_id, type, reference, status, created_at,
		  submitted_at, flowcore_definition_id, subject_reference)
		 values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		submission.ID, submission.SessionID, submission.Type, submission.Reference,
		submission.Status, submission.CreatedAt, submission.SubmittedAt,
		submission.FlowcoreDefinitionID, submission.SubjectReference)

	return err
}

func (s *Store) Submissions(ctx context.Context, sessionID string) ([]Submission, error) {
	rows, err := s.pool.Query(ctx,
		`select `+submissionColumns+` from casework.submission
		 where session_id = $1 order by created_at desc`,
		sessionID)
	if err != nil {
		return nil, err
	}

	return pgx.CollectRows(rows, scanSubmission)
}

// SubmissionByReference finds one within a session. Every read is session-scoped:
// tenancy is CaseWork's job, because the library has no notion of it.
func (s *Store) SubmissionByReference(ctx context.Context, sessionID, reference string) (Submission, error) {
	rows, err := s.pool.Query(ctx,
		`select `+submissionColumns+` from casework.submission
		 where session_id = $1 and reference = $2`,
		sessionID, reference)
	if err != nil {
		return Submission{}, err
	}

	submission, err := pgx.CollectExactlyOneRow(rows, scanSubmission)
	if errors.Is(err, pgx.ErrNoRows) {
		return Submission{}, ErrNotFound
	}

	return submission, err
}

// MarkSubmitted records that a run has begun. It is the only thing that moves a
// submission out of draft, and it stamps the workflow the run started under.
func (s *Store) MarkSubmitted(
	ctx context.Context,
	id uuid.UUID,
	definitionID uuid.UUID,
	subjectReference string,
) error {
	_, err := s.pool.Exec(ctx,
		`update casework.submission
		 set status = 'submitted', submitted_at = $2,
		     flowcore_definition_id = $3, subject_reference = $4
		 where id = $1 and status = 'draft'`,
		id, time.Now(), definitionID, subjectReference)

	return err
}

// --- details --------------------------------------------------------------

func (s *Store) InsertClaimDetail(ctx context.Context, detail ClaimDetail) error {
	_, err := s.pool.Exec(ctx,
		`insert into casework.claim_detail
		 (submission_id, policy_number, claimant_name, amount, occurred_at, incident_narrative)
		 values ($1, $2, $3, $4, $5, $6)`,
		detail.SubmissionID, detail.PolicyNumber, detail.ClaimantName,
		detail.Amount, detail.OccurredAt, detail.IncidentNarrative)

	return err
}

func (s *Store) ClaimDetail(ctx context.Context, submissionID uuid.UUID) (ClaimDetail, error) {
	var detail ClaimDetail
	err := s.pool.QueryRow(ctx,
		`select submission_id, policy_number, claimant_name, amount, occurred_at, incident_narrative
		 from casework.claim_detail where submission_id = $1`,
		submissionID).Scan(&detail.SubmissionID, &detail.PolicyNumber, &detail.ClaimantName,
		&detail.Amount, &detail.OccurredAt, &detail.IncidentNarrative)
	if errors.Is(err, pgx.ErrNoRows) {
		return ClaimDetail{}, ErrNotFound
	}

	return detail, err
}

func (s *Store) InsertApplicationDetail(ctx context.Context, detail ApplicationDetail) error {
	_, err := s.pool.Exec(ctx,
		`insert into casework.application_detail
		 (submission_id, proposer_name, cover_type, sum_insured, disclosures)
		 values ($1, $2, $3, $4, $5)`,
		detail.SubmissionID, detail.ProposerName, detail.CoverType,
		detail.SumInsured, detail.Disclosures)

	return err
}

func (s *Store) ApplicationDetail(ctx context.Context, submissionID uuid.UUID) (ApplicationDetail, error) {
	var detail ApplicationDetail
	err := s.pool.QueryRow(ctx,
		`select submission_id, proposer_name, cover_type, sum_insured, disclosures
		 from casework.application_detail where submission_id = $1`,
		submissionID).Scan(&detail.SubmissionID, &detail.ProposerName,
		&detail.CoverType, &detail.SumInsured, &detail.Disclosures)
	if errors.Is(err, pgx.ErrNoRows) {
		return ApplicationDetail{}, ErrNotFound
	}

	return detail, err
}

// --- documents ------------------------------------------------------------

// AddDocument bumps the submission's revision and files the document at it, in
// one transaction.
//
// The two halves cannot be separated. A document that landed without moving the
// revision would be invisible to the currency rule, and a revision that moved
// without a document would make an unchanged file look edited — and both would
// be stamped on a completion as the truth about what a decision was made
// against.
//
// It is also the only way documents ever arrive. Seeding takes the same path as
// an upload, so the revision a seeded case starts at is a real count of what is
// on it rather than a number written by hand.
func (s *Store) AddDocument(ctx context.Context, document Document) (Document, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Document{}, err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	err = tx.QueryRow(ctx,
		`update casework.submission set revision = revision + 1
		 where id = $1 returning revision`,
		document.SubmissionID).Scan(&document.AddedAtRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, ErrNotFound
	}

	if err != nil {
		return Document{}, err
	}

	_, err = tx.Exec(ctx,
		`insert into casework.document
		 (id, submission_id, name, kind, received_at, body, source_file, added_at_revision)
		 values ($1, $2, $3, $4, $5, $6, $7, $8)`,
		document.ID, document.SubmissionID, document.Name, document.Kind,
		document.ReceivedAt, document.Body, document.SourceFile, document.AddedAtRevision)
	if err != nil {
		return Document{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Document{}, err
	}

	return document, nil
}

// Documents returns every document on a submission, superseded ones included,
// oldest first.
//
// Nothing is filtered here. Which of them are in force is a question with a
// different answer for an agent reading the file now and for a visit that closed
// three revisions ago, so it is answered by Current at the point of asking.
func (s *Store) Documents(ctx context.Context, submissionID uuid.UUID) ([]Document, error) {
	rows, err := s.pool.Query(ctx,
		`select id, submission_id, name, kind, received_at, body, source_file, added_at_revision
		 from casework.document where submission_id = $1 order by added_at_revision, name`,
		submissionID)
	if err != nil {
		return nil, err
	}

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Document, error) {
		var document Document
		err := row.Scan(&document.ID, &document.SubmissionID, &document.Name,
			&document.Kind, &document.ReceivedAt, &document.Body, &document.SourceFile,
			&document.AddedAtRevision)

		return document, err
	})
}

// --- the workflow registry ------------------------------------------------

func (s *Store) RegisterWorkflow(ctx context.Context, workflow RegisteredWorkflow) error {
	_, err := s.pool.Exec(ctx,
		`insert into casework.workflow_registry
		 (id, session_id, submission_type, name, flowcore_definition_id, active, created_at)
		 values ($1, $2, $3, $4, $5, $6, $7)`,
		workflow.ID, workflow.SessionID, workflow.SubmissionType, workflow.Name,
		workflow.FlowcoreDefinitionID, workflow.Active, workflow.CreatedAt)

	return err
}

const registryColumns = `id, session_id, submission_type, name,
	flowcore_definition_id, active, created_at`

func scanRegistered(row pgx.CollectableRow) (RegisteredWorkflow, error) {
	var workflow RegisteredWorkflow
	err := row.Scan(&workflow.ID, &workflow.SessionID, &workflow.SubmissionType,
		&workflow.Name, &workflow.FlowcoreDefinitionID, &workflow.Active, &workflow.CreatedAt)

	return workflow, err
}

func (s *Store) RegisteredWorkflows(ctx context.Context, sessionID string) ([]RegisteredWorkflow, error) {
	rows, err := s.pool.Query(ctx,
		`select `+registryColumns+` from casework.workflow_registry
		 where session_id = $1 order by created_at desc`,
		sessionID)
	if err != nil {
		return nil, err
	}

	return pgx.CollectRows(rows, scanRegistered)
}

// ActiveWorkflow answers the question FlowCore cannot: which definition should a
// submission of this type start under?
func (s *Store) ActiveWorkflow(ctx context.Context, sessionID string, submissionType SubmissionType) (RegisteredWorkflow, error) {
	rows, err := s.pool.Query(ctx,
		`select `+registryColumns+` from casework.workflow_registry
		 where session_id = $1 and submission_type = $2 and active`,
		sessionID, submissionType)
	if err != nil {
		return RegisteredWorkflow{}, err
	}

	workflow, err := pgx.CollectExactlyOneRow(rows, scanRegistered)
	if errors.Is(err, pgx.ErrNoRows) {
		return RegisteredWorkflow{}, ErrNotFound
	}

	return workflow, err
}

// --- document types --------------------------------------------------------

const documentTypeColumns = `id, session_id, name, title, pass_finding, fail_finding, created_at`

func scanDocumentType(row pgx.CollectableRow) (DocumentType, error) {
	var documentType DocumentType
	err := row.Scan(&documentType.ID, &documentType.SessionID, &documentType.Name,
		&documentType.Title, &documentType.PassFinding, &documentType.FailFinding,
		&documentType.CreatedAt)

	return documentType, err
}

// EnsureDocumentType writes a type if the session does not have one by that name,
// and returns the id either way.
//
// Idempotent because a type can belong to more than one scenario —
// `correspondence` is on both claims and policy applications — and the two are
// seeded separately. Which of them "owns" it is not a question worth having: a
// document type is a type, and being expected by steps of two workflows is
// ordinary.
func (s *Store) EnsureDocumentType(ctx context.Context, documentType DocumentType) (uuid.UUID, error) {
	var id uuid.UUID

	err := s.pool.QueryRow(ctx,
		`insert into casework.document_type (`+documentTypeColumns+`)
		 values ($1, $2, $3, $4, $5, $6, $7)
		 on conflict (session_id, name) do update set name = excluded.name
		 returning id`,
		documentType.ID, documentType.SessionID, documentType.Name, documentType.Title,
		documentType.PassFinding, documentType.FailFinding, documentType.CreatedAt).Scan(&id)

	return id, err
}

// DocumentTypes is every type this session knows about, for the upload form's
// kind selector — which offers all of them, because you file whatever arrived.
func (s *Store) DocumentTypes(ctx context.Context, sessionID string) ([]DocumentType, error) {
	rows, err := s.pool.Query(ctx,
		`select `+documentTypeColumns+` from casework.document_type
		 where session_id = $1 order by title`,
		sessionID)
	if err != nil {
		return nil, err
	}

	return pgx.CollectRows(rows, scanDocumentType)
}

// AttachDocumentType records that a step expects a kind of document.
func (s *Store) AttachDocumentType(
	ctx context.Context,
	definitionID uuid.UUID,
	stepDefinitionID uuid.UUID,
	documentTypeID uuid.UUID,
) error {
	_, err := s.pool.Exec(ctx,
		`insert into casework.step_document_type
		 (flowcore_definition_id, step_definition_id, document_type_id)
		 values ($1, $2, $3) on conflict do nothing`,
		definitionID, stepDefinitionID, documentTypeID)

	return err
}

// DocumentTypesForStep is what one step expects.
//
// Keyed by the step definition id rather than the step's name, because a name is
// something the workflow editor can change and metadata keyed to one orphans
// without anything failing. FlowCore exposes the definition id on a running step.
func (s *Store) DocumentTypesForStep(
	ctx context.Context,
	sessionID string,
	stepDefinitionID uuid.UUID,
) ([]DocumentType, error) {
	rows, err := s.pool.Query(ctx,
		`select t.id, t.session_id, t.name, t.title, t.pass_finding, t.fail_finding, t.created_at
		 from casework.document_type t
		 join casework.step_document_type a on a.document_type_id = t.id
		 where t.session_id = $1 and a.step_definition_id = $2
		 order by t.title`,
		sessionID, stepDefinitionID)
	if err != nil {
		return nil, err
	}

	return pgx.CollectRows(rows, scanDocumentType)
}

// DocumentTypesForDefinition is every type any step of a workflow expects.
//
// This is what answers "which documents belong to a claim" without a column
// saying so: a type is a claim document because a step of the claim workflow
// reads it. It is what the picker falls back to on a draft, which has no running
// step to ask about.
func (s *Store) DocumentTypesForDefinition(
	ctx context.Context,
	sessionID string,
	definitionID uuid.UUID,
) ([]DocumentType, error) {
	rows, err := s.pool.Query(ctx,
		`select distinct t.id, t.session_id, t.name, t.title, t.pass_finding,
		        t.fail_finding, t.created_at
		 from casework.document_type t
		 join casework.step_document_type a on a.document_type_id = t.id
		 where t.session_id = $1 and a.flowcore_definition_id = $2
		 order by t.title`,
		sessionID, definitionID)
	if err != nil {
		return nil, err
	}

	return pgx.CollectRows(rows, scanDocumentType)
}
