package ingestion

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf" // swap for your preferred PDF lib if different

	"github.com/mtariq99/retrievion/models"
)

type Parser interface {
	Parse(ctx context.Context, r io.Reader, mimeType string) (*models.ParsedDocument, error)
}

type PDFParser struct{}

func NewPDFParser() *PDFParser {
	return &PDFParser{}
}

func (p *PDFParser) Parse(ctx context.Context, r io.Reader, mimeType string) (*models.ParsedDocument, error) {
	if mimeType != "application/pdf" {
		return nil, fmt.Errorf("unsupported mime type for parser: %s", mimeType)
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read pdf bytes: %w", err)
	}

	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open pdf: %w", err)
	}

	doc := &models.ParsedDocument{}
	var full strings.Builder

	numPages := reader.NumPage()
	for i := 1; i <= numPages; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		page := reader.Page(i)
		if page.V.IsNull() {
			continue
		}

		text, err := page.GetPlainText(nil)
		if err != nil {
			continue // skip unreadable pages rather than failing the whole document
		}

		text = normalizeText(text)
		if text == "" {
			continue
		}

		doc.Pages = append(doc.Pages, models.Page{Number: i, Content: text})
		full.WriteString(text)
		full.WriteString("\n\n")
	}

	if len(doc.Pages) == 0 {
		return nil, fmt.Errorf("no extractable text found in pdf")
	}

	doc.Content = full.String()
	return doc, nil
}

func normalizeText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	var out []string
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return strings.Join(out, "\n")
}
