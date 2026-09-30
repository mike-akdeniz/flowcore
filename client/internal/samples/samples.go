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

// Outcome is what a sample document argues for at the step that judges it.
//
// Two words, and deliberately only two. There were eight — complete/incomplete,
// consistent/contradicts, simple/complex, clean/adverse — one private pair per
// agent step, and every new step wanted a ninth and tenth. `pass` and `fail` say
// the one thing a sample needs to say: whether the step it reaches will be
// satisfied. A claim that passes triage takes the fast track; one that fails
// needs full assessment. Triage is a screen, so the words fit it as well as they
// fit the risk screen.
//
// It labels the sample in the picker — "Repair estimate — pass" — so a visitor
// with an API key can choose which way to push a real model. Without a key
// nothing reads it: the simulated checker follows fixed branches (client
// decision 39).
type Outcome string

const (
	// OutcomeNone is a document whose name carries no outcome — anything a
	// visitor uploaded themselves.
	OutcomeNone Outcome = ""
	OutcomePass Outcome = "pass"
	OutcomeFail Outcome = "fail"
)

// Outcomes is the vocabulary.
var Outcomes = []Outcome{OutcomePass, OutcomeFail}

// Document is one sample: its file name, the document type it is an example of,
// what it argues for, and its text.
//
// No title and no submission type. Both used to be decided here by a switch on
// the file name's prefix, and both now live in the database — the title on the
// type row, and which kinds of case may hold it on the allowed lists. What is left is
// structural: this package knows the *shape* of a sample's name and nothing about
// any particular kind of document.
type Document struct {
	FileName string
	// Kind is the document type's name, which is the file name's middle segment
	// verbatim: `estimate`, `police-report`, `prior-insurer`. One spelling across
	// the file, the row and the document.
	Kind    string
	Outcome Outcome
	Body    string
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

	// Grouped by document type, then by the number the file name carries. The
	// numbering restarts for each kind of submission, and the picker only ever
	// shows one kind at a time, so grouping keeps each list reading from one.
	//
	// By the number and not the name: a string sort puts "10-" before "9-", which
	// breaks the one thing the prefix is for, and only once a tenth sample exists.
	sort.Slice(library.documents, func(i, j int) bool {
		left, right := library.documents[i], library.documents[j]
		if order(left.FileName) != order(right.FileName) {
			return order(left.FileName) < order(right.FileName)
		}

		return left.FileName < right.FileName
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

// parse splits `<order>-<type>-<outcome>.txt` into its parts.
//
// Entirely structural: strip the number, strip the outcome, and whatever is left
// is the document type's name. There is no list of kinds here, so adding one is
// two files and a row rather than a code change.
//
// A name that does not match the convention still loads — it is simply a document
// with no outcome, which is exactly what an uploaded file is.
func parse(fileName, body string) Document {
	stem := TrimOrder(strings.TrimSuffix(fileName, ".txt"))

	document := Document{FileName: fileName, Body: body, Kind: stem}

	for _, outcome := range Outcomes {
		suffix := "-" + string(outcome)
		if strings.HasSuffix(stem, suffix) {
			document.Outcome = outcome
			document.Kind = strings.TrimSuffix(stem, suffix)

			break
		}
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
