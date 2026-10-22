package models

import (
	"errors"
	"fmt"
	"strings"
)

// sectionSeparator joins section contents into ParsedDocument.Content.
const sectionSeparator = "\n\n"

// Section is one contiguous, citable unit of a parsed document, in reading
// order. What a Section corresponds to depends on the source format:
//
//	PDF  -> one physical page   (Page set, Title empty)
//	DOCX -> one heading block   (Title set, Page nil)
//
// Downstream stages (chunker, embedder, citations) must treat these
// differences as optional metadata, never as structure they depend on.
type Section struct {
	// Title is the heading path for the section, e.g.
	// "Work Experience > Backend Engineer — Example Company". Empty when the
	// source has no heading for this block (PDF pages, DOCX preamble).
	Title string

	// Page is the 1-based physical page number. It is non-nil ONLY when the
	// format has real, reliable pagination (PDF). DOCX is reflowed by the
	// renderer, so a DOCX parser must leave this nil rather than guess.
	Page *int

	// Content is the normalized text of the section, one paragraph per line.
	// When Title is set, the heading line is the first line of Content so the
	// text remains meaningful when chunked or embedded on its own.
	Content string
}

// ParsedDocument is the format-independent output of every ingestion.Parser.
//
// Invariants (checked by Validate, relied on by the chunker):
//   - Sections is non-empty and in reading order.
//   - Every Section.Content is non-blank and already trimmed.
//   - Section.Page is nil or > 0.
//   - Content == the section contents joined with "\n\n".
//
// Warnings records non-fatal extraction problems (e.g. one PDF page that
// failed to extract) so partial results are never silently lossy.
type ParsedDocument struct {
	Content  string
	Sections []Section
	Warnings []string
}

// NewParsedDocument builds a ParsedDocument, dropping blank sections and
// deriving Content. Every parser should construct its result through it so
// the invariants hold by construction.
func NewParsedDocument(sections []Section, warnings []string) *ParsedDocument {
	doc := &ParsedDocument{Warnings: warnings}
	var b strings.Builder
	for _, s := range sections {
		s.Content = strings.TrimSpace(s.Content)
		if s.Content == "" {
			continue
		}
		s.Title = strings.TrimSpace(s.Title)
		if len(doc.Sections) > 0 {
			b.WriteString(sectionSeparator)
		}
		b.WriteString(s.Content)
		doc.Sections = append(doc.Sections, s)
	}
	doc.Content = b.String()
	return doc
}

// Validate reports the first violated invariant, or nil. Parsers' tests use it
// to prove PDF and DOCX output satisfy the same contract.
func (d *ParsedDocument) Validate() error {
	if d == nil {
		return errors.New("parsed document is nil")
	}
	if len(d.Sections) == 0 {
		return errors.New("parsed document has no sections")
	}
	contents := make([]string, len(d.Sections))
	for i, s := range d.Sections {
		if strings.TrimSpace(s.Content) == "" {
			return fmt.Errorf("section %d has blank content", i)
		}
		if s.Content != strings.TrimSpace(s.Content) {
			return fmt.Errorf("section %d content is not trimmed", i)
		}
		if s.Page != nil && *s.Page <= 0 {
			return fmt.Errorf("section %d has invalid page %d", i, *s.Page)
		}
		contents[i] = s.Content
	}
	if want := strings.Join(contents, sectionSeparator); d.Content != want {
		return errors.New("Content does not equal the joined section contents")
	}
	return nil
}
