package store

import (
	"testing"

	"github.com/google/uuid"
)

// Current carries the rule client decision 20 turns on, and it needs no database
// to exercise: documents are superseded per kind, never replaced, and which ones
// are in force depends on the revision being asked about.
func TestCurrent(t *testing.T) {
	estimateOld := document("estimate", 3)
	report := document("police_report", 4)
	photograph := document("photograph", 5)
	estimateNew := document("estimate", 6)

	onFile := []Document{estimateOld, report, photograph, estimateNew}

	t.Run("the newest of each kind is in force", func(t *testing.T) {
		assertKinds(t, Current(onFile, 6),
			map[string]uuid.UUID{
				"estimate":      estimateNew.ID,
				"police_report": report.ID,
				"photograph":    photograph.ID,
			})
	})

	t.Run("an earlier revision sees what was in force then", func(t *testing.T) {
		// This is what keeps a completed visit legible. The decision stamped
		// revision 5, so it read the estimate that was on the claim at the time,
		// not the one that arrived afterwards and changed the answer.
		assertKinds(t, Current(onFile, 5),
			map[string]uuid.UUID{
				"estimate":      estimateOld.ID,
				"police_report": report.ID,
				"photograph":    photograph.ID,
			})
	})

	t.Run("kinds do not displace one another", func(t *testing.T) {
		// The reason a step reading several kinds at once needs no tie-break.
		if got := len(Current(onFile, 6)); got != 3 {
			t.Errorf("current documents = %d, want 3", got)
		}
	})

	t.Run("a revision before anything arrived is empty", func(t *testing.T) {
		if got := Current(onFile, 1); len(got) != 0 {
			t.Errorf("current documents = %d, want 0", len(got))
		}
	})

	t.Run("order follows arrival", func(t *testing.T) {
		current := Current(onFile, 6)
		for i := 1; i < len(current); i++ {
			if current[i-1].AddedAtRevision > current[i].AddedAtRevision {
				t.Fatalf("current documents are out of order at %d", i)
			}
		}
	})
}

// document files one of a kind, identified by a type id derived from the kind's
// name so the test can keep naming kinds as words.
func document(kind string, revision int) Document {
	return Document{
		ID:              uuid.Must(uuid.NewV7()),
		DocumentTypeID:  uuid.NewSHA1(uuid.NameSpaceOID, []byte(kind)),
		Kind:            kind,
		AddedAtRevision: revision,
	}
}

func assertKinds(t *testing.T, current []Document, want map[string]uuid.UUID) {
	t.Helper()

	if len(current) != len(want) {
		t.Fatalf("current documents = %d, want %d", len(current), len(want))
	}

	for _, document := range current {
		if want[document.Kind] != document.ID {
			t.Errorf("current %s = %v, want %v", document.Kind, document.ID, want[document.Kind])
		}
	}
}
