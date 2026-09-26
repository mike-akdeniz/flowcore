// Package samples is the library of example documents a visitor can add to a
// case.
//
// They exist on disk in `sample-documents/` so anyone reading the repository can
// see and edit them, and they are embedded in the binary so a hosted visitor —
// who has no folder to browse — gets the same set from the application itself.
//
// The file name is a convention with two jobs. It tells a person what the
// document argues for, and when no model is configured it is the *only* thing the
// simulated agent steps have to go on.
package samples

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// Outcome is what a sample document argues for, taken from its file name.
//
// One word per outcome across every kind of document, so the simulated checker
// reads a fixed vocabulary rather than a list of synonyms.
type Outcome string

const (
	// OutcomeNone is a document whose name carries no signal — anything a visitor
	// uploaded themselves. Without a model there is nothing to read it with, so a
	// step judging it is simulated at random, and the application says so.
	OutcomeNone Outcome = ""
	// Complete and Incomplete are for `documentation check`: whether the file has
	// what an assessor needs.
	OutcomeComplete   Outcome = "complete"
	OutcomeIncomplete Outcome = "incomplete"
	// Consistent and Contradicts are for `narrative consistency`: whether a
	// document agrees with the claimant's own account.
	OutcomeConsistent  Outcome = "consistent"
	OutcomeContradicts Outcome = "contradicts"
	// Simple and Complex are for `triage`, the first step every claim meets.
	// Without them the demonstration's opening move is a coin flip.
	OutcomeSimple  Outcome = "simple"
	OutcomeComplex Outcome = "complex"
)

// Outcomes is the vocabulary, longest-matching first so that a name ending in
// "-incomplete" is not read as "-complete".
var Outcomes = []Outcome{
	OutcomeIncomplete, OutcomeComplete,
	OutcomeConsistent, OutcomeContradicts,
	OutcomeSimple, OutcomeComplex,
}

// Document is one sample: its file name, the kind of document it represents, what
// it argues for, and its text.
type Document struct {
	FileName string
	Kind     string
	Outcome  Outcome
	// Title is what the document is called in a case file, derived from the kind.
	Title string
	Body  string
}

// Library is the loaded set.
type Library struct {
	documents []Document
	byName    map[string]Document
}

// Load reads every .txt in the given filesystem.
func Load(files fs.FS) (*Library, error) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return nil, err
	}

	library := &Library{byName: map[string]Document{}}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".txt") {
			continue
		}

		body, err := fs.ReadFile(files, entry.Name())
		if err != nil {
			return nil, err
		}

		document := parse(entry.Name(), string(body))
		library.documents = append(library.documents, document)
		library.byName[document.FileName] = document
	}

	sort.Slice(library.documents, func(i, j int) bool {
		return library.documents[i].FileName < library.documents[j].FileName
	})

	return library, nil
}

func (l *Library) All() []Document { return l.documents }

func (l *Library) ByName(fileName string) (Document, bool) {
	document, ok := l.byName[fileName]

	return document, ok
}

// MustHave returns a sample by name, for seeding. A missing one is a packaging
// error rather than a runtime condition, so it fails loudly at start-up.
func (l *Library) MustHave(fileName string) Document {
	document, ok := l.byName[fileName]
	if !ok {
		panic(fmt.Sprintf("sample document %q is missing from the embedded set", fileName))
	}

	return document
}

// parse splits `<order>-<kind>-<outcome>.txt` into its parts.
//
// A name that does not match the convention still loads — it is simply a document
// with no outcome, which is exactly what an uploaded file is.
func parse(fileName, body string) Document {
	stem := TrimOrder(strings.TrimSuffix(fileName, ".txt"))

	document := Document{FileName: fileName, Body: body, Kind: "correspondence"}

	for _, outcome := range Outcomes {
		suffix := "-" + string(outcome)
		if strings.HasSuffix(stem, suffix) {
			document.Outcome = outcome
			stem = strings.TrimSuffix(stem, suffix)

			break
		}
	}

	// The kind is the axis currency turns on: a newer document of a kind
	// supersedes an older one of the same kind, so a witness statement and an
	// intake note cannot share `correspondence` without displacing each other.
	switch {
	case strings.HasPrefix(stem, "police-report"):
		document.Kind = "police_report"
		document.Title = "Police report"
	case strings.HasPrefix(stem, "estimate"):
		document.Kind = "estimate"
		document.Title = "Repair estimate"
	case strings.HasPrefix(stem, "witness-statement"):
		document.Kind = "witness_statement"
		document.Title = "Witness statement"
	case strings.HasPrefix(stem, "intake-note"):
		document.Kind = "intake_note"
		document.Title = "Intake note"
	default:
		document.Title = titleFrom(stem)
	}

	return document
}

// TrimOrder strips the leading `<n>-` that recommends the order to try the
// samples in.
//
// The number is guidance for a person reading the folder and nothing else: it
// sorts the list so the most useful document is offered first. It carries no
// meaning for the application, which is why it comes off before anything here
// looks at the name.
func TrimOrder(stem string) string {
	digits := 0
	for digits < len(stem) && stem[digits] >= '0' && stem[digits] <= '9' {
		digits++
	}

	if digits == 0 || digits == len(stem) || stem[digits] != '-' {
		return stem
	}

	return stem[digits+1:]
}

func titleFrom(stem string) string {
	words := strings.ReplaceAll(stem, "-", " ")
	if words == "" {
		return "Document"
	}

	return strings.ToUpper(words[:1]) + words[1:]
}
