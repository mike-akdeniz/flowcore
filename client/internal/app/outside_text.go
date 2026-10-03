package app

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Outside text is whatever a visitor typed or uploaded: a case's fields and its
// documents. All of it ends up in front of a model, and none of it can be kept
// from instructing one — a sentence is data and instruction at once, so there is
// nothing to escape the way a query escapes a quote. What can be done is done
// here and in the prompt (client decision 65): the person reading a case and the
// model deciding it see the same text, a field that should be one line is one
// line, and outside text cannot pass itself off as the prompt's own structure.

// Limits in characters. Generous for anything a case needs — the longest sample
// document is under 900 — and they bound what one case can cost a hosted model.
const (
	lineLimit     = 120
	proseLimit    = 10000
	documentLimit = 20000
)

// clean removes what a reader cannot see and a model still reads: format
// characters — zero-width spaces and joiners, direction overrides, and the tag
// block, which spells out whole hidden sentences — and control characters other
// than newline and tab. Stored cleaned, so the case screen shows what the model
// was given.
func clean(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")

	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}

		if unicode.Is(unicode.Cf, r) || unicode.IsControl(r) {
			return -1
		}

		return r
	}, text)
}

// CleanLine cleans a field that should be a single line — a name, a policy
// number, a file name — and refuses one that is not. On one line and short, a
// field cannot carry a paragraph of instructions or forge a heading in the case
// text.
func CleanLine(label, value string) (string, error) {
	value = strings.TrimSpace(clean(value))
	if strings.Contains(value, "\n") {
		return "", fmt.Errorf("%s must be a single line", label)
	}

	if utf8.RuneCountInString(value) > lineLimit {
		return "", fmt.Errorf("%s is longer than %d characters", label, lineLimit)
	}

	return value, nil
}

// cleanProse cleans free text — an account, disclosures, a document — and
// refuses one longer than its limit.
func cleanProse(label, value string, limit int) (string, error) {
	value = strings.TrimSpace(clean(value))
	if utf8.RuneCountInString(value) > limit {
		return "", fmt.Errorf("%s is longer than %d characters", label, limit)
	}

	return value, nil
}

// CleanDocument is cleanProse at the document limit.
func CleanDocument(label, value string) (string, error) {
	return cleanProse(label, value, documentLimit)
}

// tagLike is anything outside text could use to open or close a tag.
var tagLike = regexp.MustCompile(`<(/?[A-Za-z])`)

// quoted prepares outside text for the case text a model reads. The prompt marks
// where each piece of it begins and ends with tags, and text that could write a
// tag could close its own and forge what follows — a second, clean letter from
// the previous insurer, say. Its angle bracket becomes a lookalike, which a
// model reads as the same word but not as structure.
//
// Only for the prompt. What is stored and shown on the case screen is the text
// as filed.
func quoted(text string) string {
	return tagLike.ReplaceAllString(text, "‹$1")
}
