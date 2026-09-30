package app

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mike-akdeniz/flowcore"
	"github.com/mike-akdeniz/flowcore/client/internal/store"
)

// The example document types, which kinds of case may hold them, and which steps
// require them.
//
// This file is the one place a kind of document is described. It used to be four:
// a prefix switch in the sample parser, a map of which step read which kind, a
// map of canned findings, and a SQL CHECK constraint listing them — keyed by
// three different things, with nothing failing when they drifted apart. The rows
// below replace all four, and after seeding the database and the workflow
// definitions are the source rather than this file.
//
// `name` is the document type's name in the sample's file name and here. A sample
// called `1-estimate-pass.txt` is an example of the type named `estimate`, and
// nothing has to map between them. Everything else refers to the type by its id.

type documentType struct {
	name  string
	title string
	pass  string
	fail  string
}

// requirement names the document types a decision on a step requires, by step
// name and type name.
//
// Resolved to type ids and written onto the definition's steps before it is
// created, so FlowCore stores them and freezes them into every run. Names are
// fine *here* — this runs once, against a definition built three lines earlier —
// and would not be fine anywhere they could outlive a rename.
type requirement struct {
	step  string
	types []string
}

var claimDocumentTypes = []documentType{
	{
		name: "intake-note", title: "Intake note",
		pass: "Single vehicle, nobody to trace, and inside the fast-track limit.",
		fail: "No third party, an account the claimant did not witness, and above the fast-track limit.",
	},
	{
		name: "estimate", title: "Repair estimate",
		pass: "Itemised: parts, labour, hours, rate and VAT all stated.",
		fail: "One approximate figure. No rate, no hours, no VAT — nothing an assessor can check.",
	},
	{
		name: "police-report", title: "Police report",
		pass: "Places the vehicle at the address given, with debris consistent with the account.",
		fail: "Places the vehicle two miles away, already damaged, hours before the claimant says it happened.",
	},
	{
		name: "witness-statement", title: "Witness statement",
		pass: "Undamaged late that evening, damaged by morning, an impact heard overnight.",
		fail: "Describes the damage happening in daylight with the claimant driving.",
	},
	{
		// A photograph is a row with a name and no body, so no agent step reads
		// one and these findings are never used. The type exists so a photograph
		// can be filed at all, which is the honest reason — not every document on
		// a claim is evidence an agent can weigh.
		name: "photograph", title: "Photographs",
		pass: "Photographs are on file.",
		fail: "No photographs are on file.",
	},
	{
		name: "correspondence", title: "Correspondence",
		pass: "Supports the account given.",
		fail: "At odds with the account given.",
	},
}

var applicationDocumentTypes = []documentType{
	{
		name: "prior-insurer", title: "Previous insurer's letter",
		pass: "Four years, no claims, no convictions, lapsed at the proposer's own request.",
		fail: "Declined renewal after two convictions and a claim, and the vehicle was not garaged as declared.",
	},
	{
		name: "inspection", title: "Vehicle inspection",
		pass: "As declared: mileage, condition, security and where it is kept all match.",
		fail: "Mileage well over, undeclared modifications, and kept on the highway rather than garaged.",
	},
	{
		name: "correspondence", title: "Correspondence",
		pass: "Supports the proposal as made.",
		fail: "At odds with the proposal as made.",
	},
}

// Required means required: a step cannot be decided without them (client
// decision 36). An agent cannot file a missing document, so the agent steps' sets
// are chosen to satisfy the two rules an editor would enforce — a case enters
// `triage` only if its requirements are on file, and an agent hands another
// agent only requirements it already had (client decision 38). That is why
// `triage` requires everything its agent successors do.
//
// The human steps require nothing. A person can file what is missing before
// deciding, and the claim's `awaiting documents` loop still turns on whether the
// estimate is adequate, which is a judgment rather than a presence check.
var claimRequirements = []requirement{
	{step: "triage", types: []string{"intake-note", "estimate", "police-report"}},
	{step: "documentation check", types: []string{"estimate", "police-report"}},
	{step: "narrative consistency", types: []string{"police-report"}},
}

var applicationRequirements = []requirement{
	{step: "risk screen", types: []string{"prior-insurer"}},
}

// seedDocumentTypes writes the types and allows each of them on this kind of
// case, returning their ids by name.
//
// Every listed type is allowed, including the ones no step requires. A
// photograph or a letter is something a case may hold without any decision
// depending on it, which is why the allowed list and the requirements are two
// lists rather than one derived from the other.
func (a *App) seedDocumentTypes(
	ctx context.Context,
	sessionID string,
	submissionType store.SubmissionType,
	types []documentType,
) (map[string]uuid.UUID, error) {
	ids := make(map[string]uuid.UUID, len(types))

	for _, declared := range types {
		id, err := a.Store.EnsureDocumentType(ctx, store.DocumentType{
			ID:          uuid.Must(uuid.NewV7()),
			SessionID:   sessionID,
			Name:        declared.name,
			Title:       declared.title,
			PassFinding: declared.pass,
			FailFinding: declared.fail,
			CreatedAt:   time.Now(),
		})
		if err != nil {
			return nil, fmt.Errorf("seed document type %q: %w", declared.name, err)
		}

		if err := a.Store.AllowDocumentType(ctx, id, submissionType); err != nil {
			return nil, fmt.Errorf("seed allowed document type %q: %w", declared.name, err)
		}

		ids[declared.name] = id
	}

	return ids, nil
}

// require writes the seeded requirements onto a definition's steps, as the
// opaque type ids FlowCore will store.
func require(
	definition *flowcore.WorkflowDefinition,
	requirements []requirement,
	ids map[string]uuid.UUID,
) error {
	for _, required := range requirements {
		found := false

		for i := range definition.Steps {
			if definition.Steps[i].Name != required.step {
				continue
			}

			for _, name := range required.types {
				id, ok := ids[name]
				if !ok {
					return fmt.Errorf("seed requirement: no document type named %q", name)
				}

				definition.Steps[i].RequiredInputTypeIDs = append(
					definition.Steps[i].RequiredInputTypeIDs, id.String())
			}

			found = true
		}

		if !found {
			return fmt.Errorf("seed requirement: %q has no step named %q", definition.Name, required.step)
		}
	}

	return nil
}
