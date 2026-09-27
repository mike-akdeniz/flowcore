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
	"math"
	"sort"
	"strconv"
	"strings"
)

// Outcome is what a sample document does to the check it is put in front of.
//
// Two words, and deliberately only two. There were eight — complete/incomplete,
// consistent/contradicts, simple/complex, clean/adverse — one private pair per
// agent step, and every new step wanted a ninth and tenth. `pass` and `fail` say
// the one thing a sample needs to say: whether the step it reaches will be
// satisfied. A claim that passes triage takes the fast track; one that fails
// needs full assessment. Triage is a screen, so the words fit it as well as they
// fit the risk screen.
//
// The *reason* is no longer in the file name. It is in the finding the simulated
// checker stamps on the visit, where it can be a sentence about the claim rather
// than a word squeezed into a file name.
type Outcome string

const (
	// OutcomeNone is a document whose name carries no outcome — anything a
	// visitor uploaded themselves. Without a model there is nothing to read it
	// with, so a step judging it is simulated at random, and the application
	// says so.
	OutcomeNone Outcome = ""
	OutcomePass Outcome = "pass"
	OutcomeFail Outcome = "fail"
)

// Outcomes is the vocabulary.
var Outcomes = []Outcome{OutcomePass, OutcomeFail}

// Document is one sample: its file name, the kind of document it represents, what
// it argues for, and its text.
type Document struct {
	FileName string
	Kind     string
	Outcome  Outcome
	// Title is what the document is called in a case file, derived from the kind.
	Title string
	Body  string
	// AppliesTo is the submission type this sample was written for, "claim" or
	// "application". Empty means it fits either.
	//
	// It is not a rule about what a case may hold — nothing stops a police report
	// sitting on a proposal, and CaseWork does not police that. It only keeps the
	// picker to the samples that can do something, since these are documents we
	// wrote for the demonstration and we know which scenario each belongs to.
	AppliesTo string
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

	// Grouped by submission type, then by number — the numbering restarts for each
	// type, because the picker only ever shows one type's samples at a time and a
	// list that began at nine would be odd.
	//
	// By the number and not the name: a string sort puts "10-" before "9-", which
	// breaks the one thing the prefix is for, and only once a tenth sample exists.
	sort.Slice(library.documents, func(i, j int) bool {
		left, right := library.documents[i], library.documents[j]
		if left.AppliesTo != right.AppliesTo {
			return left.AppliesTo < right.AppliesTo
		}

		if order(left.FileName) != order(right.FileName) {
			return order(left.FileName) < order(right.FileName)
		}

		return left.FileName < right.FileName
	})

	return library, nil
}

func (l *Library) All() []Document { return l.documents }

// For returns the samples written for a submission type, plus any that fit
// either.
func (l *Library) For(submissionType string) []Document {
	matching := make([]Document, 0, len(l.documents))
	for _, document := range l.documents {
		if document.AppliesTo == "" || document.AppliesTo == submissionType {
			matching = append(matching, document)
		}
	}

	return matching
}

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
		document.AppliesTo = "claim"
	case strings.HasPrefix(stem, "estimate"):
		document.Kind = "estimate"
		document.Title = "Repair estimate"
		document.AppliesTo = "claim"
	case strings.HasPrefix(stem, "witness-statement"):
		document.Kind = "witness_statement"
		document.Title = "Witness statement"
		document.AppliesTo = "claim"
	case strings.HasPrefix(stem, "intake-note"):
		document.Kind = "intake_note"
		document.Title = "Intake note"
		document.AppliesTo = "claim"
	case strings.HasPrefix(stem, "inspection"):
		document.Kind = "inspection_report"
		document.Title = "Vehicle inspection"
		document.AppliesTo = "application"
	case strings.HasPrefix(stem, "prior-insurer"):
		document.Kind = "prior_insurer_letter"
		document.Title = "Previous insurer's letter"
		document.AppliesTo = "application"
	default:
		document.Title = titleFrom(stem)
	}

	return document
}

// order reads the leading `<n>-`. A file without one sorts last, since the
// numbers are a recommendation and an unnumbered file is not part of it.
func order(fileName string) int {
	stem := strings.TrimSuffix(fileName, ".txt")

	digits := 0
	for digits < len(stem) && stem[digits] >= '0' && stem[digits] <= '9' {
		digits++
	}

	if digits == 0 {
		return math.MaxInt
	}

	number, err := strconv.Atoi(stem[:digits])
	if err != nil {
		return math.MaxInt
	}

	return number
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
