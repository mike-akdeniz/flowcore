package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/mike-akdeniz/flowcore/client/internal/store"
)

// SubjectView is what an agent step reads, and the revision it was read at.
//
// The revision travels with the text because the two are one fact. FlowCore
// stamps it on the completion as the subject version token, so a decision and
// the state of the file it was made against are recorded together — which is the
// only thing that tells a second visit to a step apart from the first.
type SubjectView struct {
	Text string
	// Documents are the current documents, in arrival order. Superseded ones are
	// not here: an agent reads what the file says now, exactly as a model would.
	Documents []CaseDocument
	Revision  int
}

// SubjectText renders a submission as the prose an agent step reads.
//
// This is CaseWork's whole contribution to an AI step. FlowCore holds an
// opaque reference — "s7f3a2:claim:C-1042" — and nothing else, so the material a
// model needs in order to judge anything has to be assembled here, from the
// CaseWork's own tables.
//
// It is also why the library cannot call a model itself, and not merely why it
// chooses not to: it does not have this text and could not obtain it without
// either storing subjects or calling back into its caller.
func (a *App) SubjectText(ctx context.Context, sessionID, reference string) (SubjectView, error) {
	kind, _, found := strings.Cut(reference, ":")
	if !found {
		return SubjectView{}, fmt.Errorf("malformed subject reference %q", reference)
	}

	submission, err := a.Store.SubmissionByReference(ctx, sessionID, referenceOf(reference))
	if err != nil {
		return SubjectView{}, err
	}

	switch store.SubmissionType(kind) {
	case store.TypeClaim:
		return a.claimView(ctx, submission)
	case store.TypeApplication:
		return a.applicationView(ctx, submission)
	}

	return SubjectView{}, fmt.Errorf("unknown submission kind %q", kind)
}

func (a *App) claimView(ctx context.Context, submission store.Submission) (SubjectView, error) {
	detail, err := a.Store.ClaimDetail(ctx, submission.ID)
	if err != nil {
		return SubjectView{}, err
	}

	documents, err := a.Store.Documents(ctx, submission.ID)
	if err != nil {
		return SubjectView{}, err
	}

	var text strings.Builder
	fmt.Fprintf(&text, "Claim: %s\nPolicy: %s\nClaimant: %s\nAmount: %s\nDate of incident: %s\n\n",
		submission.Reference, detail.PolicyNumber, detail.ClaimantName,
		detail.Amount, detail.OccurredAt.Format("2 January 2006"))

	fmt.Fprintf(&text, "Claimant's account:\n%s\n\n", detail.IncidentNarrative)

	return withDocuments(&text, documents, submission.Revision), nil
}

func (a *App) applicationView(ctx context.Context, submission store.Submission) (SubjectView, error) {
	detail, err := a.Store.ApplicationDetail(ctx, submission.ID)
	if err != nil {
		return SubjectView{}, err
	}

	documents, err := a.Store.Documents(ctx, submission.ID)
	if err != nil {
		return SubjectView{}, err
	}

	var text strings.Builder
	fmt.Fprintf(&text, "Application: %s\nProposer: %s\nCover: %s\nSum insured: %s\n\n",
		submission.Reference, detail.ProposerName, detail.CoverType, detail.SumInsured)

	fmt.Fprintf(&text, "Disclosures:\n%s\n\n", detail.Disclosures)

	return withDocuments(&text, documents, submission.Revision), nil
}

// withDocuments appends the file as it stands and returns the finished view.
//
// Shared by both submission types, which is the point rather than a convenience:
// a claim and a policy application have nothing in common at the detail level and
// run through the same document machinery regardless. That is the library's
// subject-agnosticism from the caller's side, and duplicating this would let the
// two drift into disagreeing about what "on file" means.
func withDocuments(text *strings.Builder, documents []store.Document, revision int) SubjectView {
	// An agent reads the file as it stands: the newest document of each kind, not
	// every estimate ever filed. The superseded ones are still on the case screen
	// — a remark that says "no labour breakdown" has to keep pointing at the
	// estimate it was about — but putting them in front of a model would be asking
	// it to weigh a document the case has already moved past.
	current := store.Current(documents, revision)

	onFile := make([]CaseDocument, 0, len(current))
	for _, document := range current {
		if document.SourceFile != nil {
			onFile = append(onFile, CaseDocument{
				Kind:     document.Kind,
				FileName: *document.SourceFile,
			})
		}
	}

	text.WriteString("Documents on file:\n")
	for _, document := range current {
		fmt.Fprintf(text, "- %s (%s, received %s)\n",
			document.Name, document.Kind, document.ReceivedAt.Format("2 January 2006"))

		// A photograph is a row with no body. The ones that carry text are what
		// an agent actually weighs against the account above — the text is the
		// document.
		if document.Body != nil {
			fmt.Fprintf(text, "  %s\n", *document.Body)
		}
	}

	if len(current) == 0 {
		text.WriteString("- nothing on file\n")
	}

	if superseded := len(documents) - len(current); superseded > 0 {
		fmt.Fprintf(text,
			"\n%d earlier document(s) on this case have been superseded by the ones above.\n",
			superseded)
	}

	return SubjectView{
		Text:      text.String(),
		Documents: onFile,
		Revision:  revision,
	}
}

// referenceOf strips the kind, leaving CaseWork's own reference:
// "claim:C-1042" becomes "C-1042".
func referenceOf(reference string) string {
	_, rest, found := strings.Cut(reference, ":")
	if !found {
		return reference
	}

	return rest
}

// SubjectReference is what FlowCore records for a submission: the session, the
// kind, and the reference. Opaque to the library, and prefixed with the session
// so two visitors working the same seeded claim have two separate runs.
func SubjectReference(sessionID string, submission store.Submission) string {
	return fmt.Sprintf("%s:%s:%s", sessionID, submission.Type, submission.Reference)
}
