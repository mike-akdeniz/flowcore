// Package samples is the library of example documents a visitor can add to a
// case.
//
// They exist on disk in `sample-documents/` so anyone reading the repository can
// see and edit them, and they are embedded in the binary so a hosted visitor —
// who has no folder to browse — gets the same set from the application itself.
//
// They are the documents of the seeded story (client decision 69), one per
// kind, so a visitor filing their own case has a document of each kind to hand.
// Agent steps read the text, never the name.
package samples

import (
	"fmt"
	"io/fs"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Document is one sample: its file name, the document type it is an example of,
// and its text.
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
	Kind string
	Body string
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

// parse reads `<order>-<type>.txt`: strip the number, and what is left is the
// document type's name. There is no list of kinds here, so adding one is a file
// and a row rather than a code change.
func parse(fileName, body string) Document {
	return Document{FileName: fileName, Body: body, Kind: TrimOrder(strings.TrimSuffix(fileName, ".txt"))}
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
