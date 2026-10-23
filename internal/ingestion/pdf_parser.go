package ingestion

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/ledongthuc/pdf"

	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/internal/errs"
	"github.com/mtariq99/retrievion/models"
)

type PDFParser struct {
	opts models.PDFOptions
}

func NewPDFParser() *PDFParser { return NewPDFParserWithOptions(models.PDFOptions{}) }

func NewPDFParserWithOptions(o models.PDFOptions) *PDFParser {
	if o.MaxInputBytes <= 0 {
		o.MaxInputBytes = int64(consts.MaxDocumentSize)
	}
	return &PDFParser{opts: o}
}

func (p *PDFParser) Parse(ctx context.Context, r io.Reader, mimeType string) (doc *models.ParsedDocument, err error) {
	if mimeType != consts.PDFMIME {
		return nil, fmt.Errorf("%w: pdf parser received %q", errs.ErrUnsupportedFileType, mimeType)
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}

	data, err := readAllLimited(r, p.opts.MaxInputBytes)
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return nil, fmt.Errorf("%w: missing %%PDF- header", errs.ErrDocumentCorrupt)
	}

	defer func() {
		if rec := recover(); rec != nil {
			doc, err = nil, fmt.Errorf("%w: pdf library panic: %v", errs.ErrDocumentCorrupt, rec)
		}
	}()

	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%w: open pdf: %w", errs.ErrDocumentCorrupt, err)
	}

	numPages := reader.NumPage()
	var (
		sections []models.Section
		warnings []string
		failed   int
	)
	for i := 1; i <= numPages; i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		text, missing, perr := extractPDFPage(reader, i)
		switch {
		case perr != nil:
			failed++
			warnings = append(warnings, fmt.Sprintf("page %d: text extraction failed: %v", i, perr))
			continue
		case missing:
			failed++
			warnings = append(warnings, fmt.Sprintf("page %d: page object missing", i))
			continue
		case text == "":
			continue
		}

		page := i
		sections = append(sections, models.Section{Page: &page, Content: text})
	}

	if len(sections) == 0 {
		if numPages > 0 && failed == numPages {
			return nil, fmt.Errorf("%w: text extraction failed on all %d pages", errs.ErrDocumentCorrupt, numPages)
		}
		return nil, fmt.Errorf("%w: no extractable text (scanned or image-only PDF? OCR is not supported)", errs.ErrDocumentEmpty)
	}
	return models.NewParsedDocument(sections, warnings), nil
}

func extractPDFPage(reader *pdf.Reader, n int) (text string, missing bool, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			text, missing, err = "", false, fmt.Errorf("panic: %v", rec)
		}
	}()
	page := reader.Page(n)
	if page.V.IsNull() {
		return "", true, nil
	}
	raw, err := page.GetPlainText(nil)
	if err != nil {
		return "", false, err
	}
	return normalizeText(raw), false, nil
}
