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
	// The claim first, so the policy application is the newer of the two and
	// leads both lists a visitor lands on — the queue and the workflows are
	// ordered newest first, so creation order is what decides.
	//
	// The policy application leads deliberately. Its workflow is three steps
	// against the claim's seven, so the whole shape of a run — submit, an agent
	// decides, a person decides, it ends — can be seen in a minute, before
	// meeting the `estimate follow-up` loop and three agent steps at once.
	//
	// Ordering the lists by kind would have done the same thing and been a lie,
	// since nothing about an application makes it sort before a claim. Seeding in
	// the order we want them read costs a comment and no query.
	if err := a.seedClaimExample(ctx, sessionID); err != nil {
		return err
	}

	return a.seedApplicationExample(ctx, sessionID)
}

func (a *App) seedClaimExample(ctx context.Context, sessionID string) error {
	// The types first: a step's requirements are type ids, so they have to exist
	// before the definition that names them is created.
	typeIDs, err := a.seedDocumentTypes(ctx, sessionID, store.TypeClaim, claimDocumentTypes)
	if err != nil {
		return err
	}

	template := claimAssessmentDefinition()
	if err := require(&template, claimRequirements, typeIDs); err != nil {
		return err
	}

	definition, err := a.Catalog.Create(ctx, template)
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
	// so there is one place to edit the text. They are the three documents
	// `triage` requires, so the claim can be submitted as it stands.
	//
	// With a model they argue for the long route. The intake note puts `triage`
	// on full assessment, the estimate is a scribbled figure `estimate check`
	// should send back for detail, and the police report contradicts the
	// claimant's account on both the circumstances and the timing, which is what
	// `narrative consistency` is for. Without a key the simulated checker takes
	// the same long route by fixed branches, except that it passes the estimate,
	// since a simulation that always took the loop back would never leave it.
	seeded := []struct {
		fileName   string
		receivedAt time.Time
	}{
		{"6-intake-note-fail.txt", date(2026, 9, 15)},
		{"7-estimate-fail.txt", date(2026, 9, 16)},
		{"8-police-report-fail.txt", date(2026, 9, 16)},
	}

	for _, entry := range seeded {
		sample := a.Samples.MustHave(entry.fileName)
		body := sample.Body
		fileName := sample.FileName

		documentType, err := a.DocumentTypeNamed(ctx, sessionID, sample.Kind)
		if err != nil {
			return err
		}

		// Seeding takes the same path as an upload, so the seeded claim's revision
		// is a real count of what is on it rather than a number written by hand.
		if _, err := a.Store.AddDocument(ctx, store.Document{
			ID:             uuid.Must(uuid.NewV7()),
			SubmissionID:   submissionID,
			Name:           documentType.Title,
			DocumentTypeID: documentType.ID,
			ReceivedAt:     entry.receivedAt,
			Body:           &body,
			SourceFile:     &fileName,
		}); err != nil {
			return err
		}
	}

	// A photograph is a row with a name, a date and no body. Real claim files
	// contain things that are not prose, and the application says so by having
	// nothing to show — there is no sample for it, because you cannot upload a
	// photograph as text.
	photographs := store.Document{
		ID:             uuid.Must(uuid.NewV7()),
		SubmissionID:   submissionID,
		Name:           "Damage photographs",
		DocumentTypeID: typeIDs["photograph"],
		ReceivedAt:     date(2026, 9, 15),
	}

	_, err = a.Store.AddDocument(ctx, photographs)

	return err
}

func (a *App) seedApplicationExample(ctx context.Context, sessionID string) error {
	typeIDs, err := a.seedDocumentTypes(ctx, sessionID, store.TypeApplication, applicationDocumentTypes)
	if err != nil {
		return err
	}

	template := underwritingDefinition()
	if err := require(&template, applicationRequirements, typeIDs); err != nil {
		return err
	}

	definition, err := a.Catalog.Create(ctx, template)
	if err != nil {
		return fmt.Errorf("seed policy assessment workflow: %w", err)
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

	if err := a.Store.InsertApplicationDetail(ctx, store.ApplicationDetail{
		SubmissionID: submissionID,
		ProposerName: "Halvard Aune",
		CoverType:    "Comprehensive motor",
		SumInsured:   "48000.00",
		Disclosures: "Two speeding convictions in the last three years, most recently March. " +
			"Vehicle is kept on the street. Business use one day a week. " +
			"A previous insurer declined cover in 2023.",
	}); err != nil {
		return err
	}

	// The letter matches the disclosures rather than contradicting them: two
	// convictions, a vehicle kept on the street, and a declinature. An adverse
	// proposal that resolved as a clean risk would be the demonstration lying
	// about itself.
	//
	// It is also the document `risk screen` requires, so the application can be
	// submitted as it stands.
	sample := a.Samples.MustHave("4-prior-insurer-fail.txt")
	body, fileName := sample.Body, sample.FileName

	documentType, err := a.DocumentTypeNamed(ctx, sessionID, sample.Kind)
	if err != nil {
		return err
	}

	_, err = a.Store.AddDocument(ctx, store.Document{
		ID:             uuid.Must(uuid.NewV7()),
		SubmissionID:   submissionID,
		Name:           documentType.Title,
		DocumentTypeID: documentType.ID,
		ReceivedAt:     date(2026, 9, 18),
		Body:           &body,
		SourceFile:     &fileName,
	})

	return err
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

// DocumentTypeNamed finds one of this session's document types by its name, the
// handle samples and the browser use.
func (a *App) DocumentTypeNamed(ctx context.Context, sessionID, name string) (store.DocumentType, error) {
	types, err := a.Store.DocumentTypes(ctx, sessionID)
	if err != nil {
		return store.DocumentType{}, err
	}

	for _, documentType := range types {
		if documentType.Name == name {
			return documentType, nil
		}
	}

	return store.DocumentType{}, fmt.Errorf("no document type named %q", name)
}
