package app

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mike-akdeniz/flowcore"
	"github.com/mike-akdeniz/flowcore/client/internal/store"
)

// The example document types, and which steps expect them.
//
// This file is the one place a kind of document is described. It used to be four:
// a prefix switch in the sample parser, a map of which step read which kind, a
// map of canned findings, and a SQL CHECK constraint listing them — keyed by
// three different things, with nothing failing when they drifted apart. The rows
// below replace all four, and after seeding the database is the source rather
// than this file.
//
// `name` is the document type's name everywhere: on the document row, in the
// sample's file name, and here. A sample called `1-estimate-pass.txt` is an
// example of the type named `estimate`, and nothing has to map between them.

type documentType struct {
	name  string
	title string
	pass  string
	fail  string
}

// expectedBy names the steps that read a type, by step name.
//
// Matched to step definition ids at seed time, from the definition the catalog
// just returned, and stored as ids. Names are fine *here* — this runs once,
// against a definition created three lines earlier — and would not be fine in the
// database, where a rename in the workflow editor would orphan them silently.
type attachment struct {
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

// `awaiting documents` expects three kinds, and that is the point of attaching
// types to steps rather than inferring them. It is a human step that reads
// nothing itself, so no rule about what a step *judges* would ever reach it — but
// it is the screen where a missing document arrives, and what is missing might be
// any of them.
var claimAttachments = []attachment{
	{step: "triage", types: []string{"intake-note"}},
	{step: "documentation check", types: []string{"estimate"}},
	{step: "awaiting documents", types: []string{"estimate", "police-report", "witness-statement"}},
	{step: "narrative consistency", types: []string{"police-report", "witness-statement"}},
}

var applicationAttachments = []attachment{
	{step: "risk screen", types: []string{"inspection", "prior-insurer"}},
}

// seedDocumentTypes writes the types and attaches them to the definition's steps.
//
// Steps with no attachment — `adjuster review`, `underwriter review` — are left
// alone deliberately. A step that declares nothing narrows nothing, and the
// picker offers everything there, because a document that has arrived has to be
// filable whatever the case is doing.
func (a *App) seedDocumentTypes(
	ctx context.Context,
	sessionID string,
	definition flowcore.WorkflowDefinition,
	types []documentType,
	attachments []attachment,
) error {
	ids := make(map[string]uuid.UUID, len(types))

	for _, declared := range types {
		if _, done := ids[declared.name]; done {
			continue
		}

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
			return fmt.Errorf("seed document type %q: %w", declared.name, err)
		}

		ids[declared.name] = id
	}

	steps := make(map[string]uuid.UUID, len(definition.Steps))
	for _, step := range definition.Steps {
		steps[step.Name] = step.ID
	}

	for _, attached := range attachments {
		stepID, ok := steps[attached.step]
		if !ok {
			return fmt.Errorf("seed attachment: %q has no step named %q",
				definition.Name, attached.step)
		}

		for _, name := range attached.types {
			typeID, ok := ids[name]
			if !ok {
				return fmt.Errorf("seed attachment: no document type named %q", name)
			}

			if err := a.Store.AttachDocumentType(ctx, definition.ID, stepID, typeID); err != nil {
				return err
			}
		}
	}

	return nil
}
