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
// This file is the one place a kind of document is described. After seeding, the
// database and the workflow definitions are the source rather than this file.
//
// `name` is the document type's name in the sample's file name and here. A sample
// called `1-estimate-pass.txt` is an example of the type named `estimate`, and
// nothing has to map between them. Everything else refers to the type by its id.

type documentType struct {
	name  string
	title string
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
	{name: "intake-note", title: "Intake note"},
	{name: "estimate", title: "Repair estimate"},
	{name: "police-report", title: "Police report"},
	{name: "witness-statement", title: "Witness statement"},
	// A photograph is a row with a name and no body. The type exists so a
	// photograph can be filed at all — not every document on a claim is text.
	{name: "photograph", title: "Photographs"},
	{name: "correspondence", title: "Correspondence"},
}

var applicationDocumentTypes = []documentType{
	{name: "prior-insurer", title: "Previous insurer's letter"},
	{name: "inspection", title: "Vehicle inspection"},
	{name: "correspondence", title: "Correspondence"},
}

// Required means required: a step cannot be decided without them (client
// decision 36). An agent cannot file a missing document, so the agent steps' sets
// are chosen to satisfy the two rules an editor would enforce — a case enters
// `triage` only if its requirements are on file, and an agent hands another
// agent only requirements it already had (client decision 38). That is why
// `triage` requires everything its agent successors do.
//
// The human steps require nothing. A person can file what is missing before
// deciding, and the claim's `estimate follow-up` loop turns on whether the
// estimate is adequate, which is a judgment rather than a presence check.
var claimRequirements = []requirement{
	{step: "triage", types: []string{"intake-note", "estimate", "police-report"}},
	{step: "estimate check", types: []string{"estimate", "police-report"}},
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
			ID:        uuid.Must(uuid.NewV7()),
			SessionID: sessionID,
			Name:      declared.name,
			Title:     declared.title,
			CreatedAt: time.Now(),
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
