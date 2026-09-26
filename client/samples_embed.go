package main

import (
	"embed"
	"io/fs"
)

// The sample documents, compiled into the binary.
//
// They live in `sample-documents/` so anyone reading the repository finds and can
// edit them, and they are embedded so a hosted visitor — who has no folder to
// browse — is offered the same set by the application. One source of text,
// two ways to reach it.
//
//go:embed sample-documents/*.txt
var sampleFS embed.FS

func sampleDocuments() (fs.FS, error) { return fs.Sub(sampleFS, "sample-documents") }
