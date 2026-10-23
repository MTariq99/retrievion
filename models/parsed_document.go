package models

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mtariq99/retrievion/internal/consts"
)

type Section struct {
	Title   string
	Page    *int
	Content string
}

type ParsedDocument struct {
	Content  string
	Sections []Section
	Warnings []string
}

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
			b.WriteString(consts.SectionSeparator)
		}
		b.WriteString(s.Content)
		doc.Sections = append(doc.Sections, s)
	}
	doc.Content = b.String()
	return doc
}

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
	if want := strings.Join(contents, consts.SectionSeparator); d.Content != want {
		return errors.New("Content does not equal the joined section contents")
	}
	return nil
}
