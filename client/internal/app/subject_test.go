package app

import (
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/mike-akdeniz/flowcore"
	"github.com/mike-akdeniz/flowcore/client/internal/store"
)

func TestDocumentReaders(t *testing.T) {
	oldEstimate := store.Document{ID: uuid.New(), Kind: "estimate", AddedAtRevision: 1}
	photograph := store.Document{ID: uuid.New(), Kind: "photograph", AddedAtRevision: 2}
	newEstimate := store.Document{ID: uuid.New(), Kind: "estimate", AddedAtRevision: 3}
	unused := store.Document{ID: uuid.New(), Kind: "correspondence", AddedAtRevision: 4}
	firstRevision, secondRevision := "2", "3"
	history := []flowcore.StepVisit{
		{StepName: "review", Completion: &flowcore.Completion{SubjectVersionToken: &firstRevision}},
		{StepName: "review", Completion: &flowcore.Completion{SubjectVersionToken: &secondRevision}},
		{StepName: "approval", Completion: &flowcore.Completion{SubjectVersionToken: &secondRevision}},
		{StepName: "open"},
	}
	readers, err := DocumentReaders(history, []store.Document{oldEstimate, photograph, newEstimate, unused})
	if err != nil {
		t.Fatal(err)
	}

	want := map[uuid.UUID][]string{
		oldEstimate.ID: {"review"},
		photograph.ID:  {"review", "approval"},
		newEstimate.ID: {"review", "approval"},
		unused.ID:      {},
	}
	if !reflect.DeepEqual(readers, want) {
		t.Fatalf("readers = %v, want %v", readers, want)
	}
}

func TestDocumentReadersRefusesUnknownRevisions(t *testing.T) {
	invalid := "not-a-revision"
	for _, revision := range []*string{nil, &invalid} {
		_, err := DocumentReaders([]flowcore.StepVisit{{
			StepName: "review", Completion: &flowcore.Completion{SubjectVersionToken: revision},
		}}, nil)
		if err == nil {
			t.Fatal("unknown historical use was accepted as unused")
		}
	}
}
