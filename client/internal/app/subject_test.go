package app

import (
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/mike-akdeniz/flowcore"
	"github.com/mike-akdeniz/flowcore/client/internal/store"
)

func TestDocumentReaders(t *testing.T) {
	estimate, photographs, correspondence := uuid.New(), uuid.New(), uuid.New()
	oldEstimate := store.Document{ID: uuid.New(), DocumentTypeID: estimate, AddedAtRevision: 1}
	photograph := store.Document{ID: uuid.New(), DocumentTypeID: photographs, AddedAtRevision: 2}
	newEstimate := store.Document{ID: uuid.New(), DocumentTypeID: estimate, AddedAtRevision: 3}
	unused := store.Document{ID: uuid.New(), DocumentTypeID: correspondence, AddedAtRevision: 4}
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

func TestSimulatedBranch(t *testing.T) {
	action := func(name string) flowcore.Action { return flowcore.Action{ID: uuid.New(), Name: name} }

	chosen, _ := simulatedBranch([]flowcore.Action{action("adequate"), action("needs detail")})
	if chosen.Name != "adequate" {
		t.Errorf("chose %q, want the demo branch that leaves the loop", chosen.Name)
	}

	chosen, _ = simulatedBranch([]flowcore.Action{action("fast track"), action("full assessment")})
	if chosen.Name != "full assessment" {
		t.Errorf("chose %q, want the demo branch", chosen.Name)
	}

	// A workflow edited in the interface names none of them.
	chosen, rule := simulatedBranch([]flowcore.Action{action("approve"), action("reject")})
	if chosen.Name != "approve" || rule != "the first of its actions" {
		t.Errorf("chose %q by %q, want the first action", chosen.Name, rule)
	}
}
