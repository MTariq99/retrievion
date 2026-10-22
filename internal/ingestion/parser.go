package ingestion

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/internal/errs"
	"github.com/mtariq99/retrievion/models"
)

type Parser interface {
	Parse(ctx context.Context, r io.Reader, mimeType string) (*models.ParsedDocument, error)
}

type ParserRegistry struct {
	parsers map[string]Parser
}

func NewParserRegistry(pdf *PDFParser, docx *DOCXParser) *ParserRegistry {
	return &ParserRegistry{parsers: map[string]Parser{
		consts.PdfMIME:  pdf,
		consts.DocxMIME: docx,
	}}
}

func (r *ParserRegistry) Parse(ctx context.Context, rd io.Reader, mimeType string) (*models.ParsedDocument, error) {
	p, ok := r.parsers[strings.ToLower(strings.TrimSpace(mimeType))]
	if !ok {
		return nil, fmt.Errorf("%w: no parser for %q", errs.ErrUnsupportedFileType, mimeType)
	}
	return p.Parse(ctx, rd, mimeType)
}

func readAllLimited(r io.Reader, max int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, fmt.Errorf("read document: %w", err)
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("%w: input exceeds %d bytes", errs.ErrDocumentTooLarge, max)
	}
	return data, nil
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
