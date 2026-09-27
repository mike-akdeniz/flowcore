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
		pass: "Single vehicle, no third party to trace, responsibility accepted and a quote " +
			"already obtained. The value is inside the fast-track limit and nothing is outstanding.",
		fail: "The claimant was not present, so the account is inference rather than observation, " +
			"there is no third party to recover from, and the value is above the fast-track limit.",
	},
	{
		name: "estimate", title: "Repair estimate",
		pass: "The estimate is itemised: parts and labour are separated, the labour rate and " +
			"hours are given, and VAT is stated. It can be checked against the damage described.",
		fail: "The estimate is a single approximate figure with no breakdown between parts and " +
			"labour, no hours or rate, and no VAT position. There is nothing here an assessor can check.",
	},
	{
		name: "police-report", title: "Police report",
		pass: "The attending officer places the vehicle at the address given, with debris in the " +
			"road consistent with an impact where it stood. Nothing in the report is inconsistent " +
			"with the claimant's account.",
		fail: "The report places the vehicle two miles from the address given, already damaged, " +
			"some hours before the claimant says the damage occurred. The account of an overnight " +
			"impact outside the home cannot both be true.",
	},
	{
		name: "witness-statement", title: "Witness statement",
		pass: "The witness saw the vehicle undamaged late in the evening and damaged the " +
			"following morning, and heard an impact overnight. This supports the claimant's " +
			"account without adding to it.",
		fail: "The witness describes the damage happening in daylight, with the claimant driving, " +
			"which contradicts the account of an overnight impact to a parked vehicle.",
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
		pass: "The correspondence supports the account given.",
		fail: "The correspondence is at odds with the account given.",
	},
}

var applicationDocumentTypes = []documentType{
	{
		name: "prior-insurer", title: "Previous insurer's letter",
		pass: "Four years of comprehensive cover with no claims and no convictions, lapsed at " +
			"the proposer's own request, and the previous insurer would have renewed.",
		fail: "The previous insurer declined to renew. Two speeding convictions during the term, " +
			"a settled damage claim, and the vehicle found kept on the highway after being " +
			"declared as garaged.",
	},
	{
		name: "inspection", title: "Vehicle inspection",
		pass: "The vehicle is as declared: mileage consistent, no undisclosed modification or " +
			"unrepaired damage, security to specification, and kept where the proposal says.",
		fail: "The vehicle differs materially from the one proposed — mileage well above the " +
			"declaration, undeclared engine and suspension modifications, an undisclosed previous " +
			"repair, and kept on the highway rather than garaged.",
	},
	{
		name: "correspondence", title: "Correspondence",
		pass: "The correspondence supports the proposal as made.",
		fail: "The correspondence is at odds with the proposal as made.",
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
