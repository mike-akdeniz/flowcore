package app

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mike-akdeniz/flowcore"
	"github.com/mike-akdeniz/flowcore/client/internal/store"
)

// SeedSession gives a visitor their own copy of the example data.
//
// Per-session copies exist because hosting means concurrent visitors: without
// them, two people signing in as Dana would work the same claim and one would
// settle it while the other was reading. CaseWork owns tenancy because
// FlowCore has none, which is the same arrangement the library's `CLAUDE.md`
// describes when it names `tenant_id` as structure with no caller.
//
// Each example is a workflow, one drafted submission, and **no run**. The
// visitor's first action is submitting the draft, so they watch the workflow
// begin rather than arriving part-way through one.
func (a *App) SeedSession(ctx context.Context, sessionID string) error {
	if err := a.seedClaimExample(ctx, sessionID); err != nil {
		return err
	}

	return a.seedApplicationExample(ctx, sessionID)
}

func (a *App) seedClaimExample(ctx context.Context, sessionID string) error {
	definition, err := a.Catalog.Create(ctx, claimAssessmentDefinition())
	if err != nil {
		return fmt.Errorf("seed claim workflow: %w", err)
	}

	if err := a.register(ctx, sessionID, store.TypeClaim, definition); err != nil {
		return err
	}

	submissionID := uuid.Must(uuid.NewV7())

	// A draft: no run, no workflow stamped, no subject reference. The schema
	// enforces that those three are absent together.
	if err := a.Store.InsertSubmission(ctx, store.Submission{
		ID:        submissionID,
		SessionID: sessionID,
		Type:      store.TypeClaim,
		Reference: "C-1042",
		Status:    "draft",
		CreatedAt: time.Now(),
	}); err != nil {
		return err
	}

	if err := a.Store.InsertClaimDetail(ctx, store.ClaimDetail{
		SubmissionID: submissionID,
		PolicyNumber: "MP-90114",
		ClaimantName: "Rosa Lindqvist",
		Amount:       "11200.00",
		OccurredAt:   date(2026, 9, 14),
		IncidentNarrative: "The car was hit while parked overnight outside my house on the 14th. " +
			"I found the damage when I came out at 07:00 and reported it straight away.",
	}); err != nil {
		return err
	}

	// The seeded file is built from the same sample documents a visitor can add,
	// so there is one place to edit the text and the simulated agent steps behave
	// deterministically along the path everyone walks.
	//
	// The three of them drive the three agent steps in order. The intake note
	// puts `triage` on the full-assessment branch rather than the fast track. The
	// estimate is a scribbled figure, so `documentation check` sends the claim to
	// `awaiting documents` and gives the visitor something to do. And the police
	// report contradicts the claimant's account on both the circumstances and the
	// timing, which is what `narrative consistency` is for once the file is
	// complete.
	//
	// Without the intake note the demonstration's opening move would be a coin
	// flip, because triage would have nothing carrying an outcome to read.
	seeded := []struct {
		fileName   string
		receivedAt time.Time
	}{
		{"6-intake-note-complex.txt", date(2026, 9, 15)},
		{"7-estimate-incomplete.txt", date(2026, 9, 16)},
		{"8-police-report-contradicts.txt", date(2026, 9, 16)},
	}

	for _, entry := range seeded {
		sample := a.Samples.MustHave(entry.fileName)
		body := sample.Body
		fileName := sample.FileName

		// Seeding takes the same path as an upload, so the seeded claim's revision
		// is a real count of what is on it rather than a number written by hand.
		if _, err := a.Store.AddDocument(ctx, store.Document{
			ID:           uuid.Must(uuid.NewV7()),
			SubmissionID: submissionID,
			Name:         sample.Title,
			Kind:         sample.Kind,
			ReceivedAt:   entry.receivedAt,
			Body:         &body,
			SourceFile:   &fileName,
		}); err != nil {
			return err
		}
	}

	// A photograph is a row with a name, a date and no body. Real claim files
	// contain things that are not prose, and the application says so by having
	// nothing to show — there is no sample for it, because you cannot upload a
	// photograph as text.
	photographs := store.Document{
		ID:           uuid.Must(uuid.NewV7()),
		SubmissionID: submissionID,
		Name:         "Damage photographs",
		Kind:         "photograph",
		ReceivedAt:   date(2026, 9, 15),
	}

	_, err = a.Store.AddDocument(ctx, photographs)

	return err
}

func (a *App) seedApplicationExample(ctx context.Context, sessionID string) error {
	definition, err := a.Catalog.Create(ctx, underwritingDefinition())
	if err != nil {
		return fmt.Errorf("seed underwriting workflow: %w", err)
	}

	if err := a.register(ctx, sessionID, store.TypeApplication, definition); err != nil {
		return err
	}

	submissionID := uuid.Must(uuid.NewV7())

	if err := a.Store.InsertSubmission(ctx, store.Submission{
		ID:        submissionID,
		SessionID: sessionID,
		Type:      store.TypeApplication,
		Reference: "P-2087",
		Status:    "draft",
		CreatedAt: time.Now(),
	}); err != nil {
		return err
	}

	return a.Store.InsertApplicationDetail(ctx, store.ApplicationDetail{
		SubmissionID: submissionID,
		ProposerName: "Halvard Aune",
		CoverType:    "Comprehensive motor",
		SumInsured:   "48000.00",
		Disclosures: "Two speeding convictions in the last three years, most recently March. " +
			"Vehicle is kept on the street. Business use one day a week. " +
			"A previous insurer declined cover in 2023.",
	})
}

// register records which FlowCore definition serves which kind of submission.
// FlowCore takes a definition id and starts a run; which definition a claim
// should use is a question only CaseWork can answer.
func (a *App) register(
	ctx context.Context,
	sessionID string,
	submissionType store.SubmissionType,
	definition flowcore.WorkflowDefinition,
) error {
	return a.Store.RegisterWorkflow(ctx, store.RegisteredWorkflow{
		ID:                   uuid.Must(uuid.NewV7()),
		SessionID:            sessionID,
		SubmissionType:       submissionType,
		Name:                 definition.Name,
		FlowcoreDefinitionID: definition.ID,
		Active:               true,
		CreatedAt:            time.Now(),
	})
}

func date(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}
