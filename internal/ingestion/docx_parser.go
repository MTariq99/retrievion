package ingestion

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/internal/errs"
	"github.com/mtariq99/retrievion/internal/filetype"
	"github.com/mtariq99/retrievion/models"
)

const (
	nsWordTransitional = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	nsWordStrict       = "http://purl.oclc.org/ooxml/wordprocessingml/main"
	nsMC               = "http://schemas.openxmlformats.org/markup-compatibility/2006"
)

type DOCXOptions struct {
	MaxInputBytes  int64
	MaxEntries     int
	MaxXMLBytes    int64
	ExcludeHeaders bool
	IncludeFooters bool
}

type DOCXParser struct {
	opts DOCXOptions
}

func NewDOCXParser() *DOCXParser { return NewDOCXParserWithOptions(DOCXOptions{}) }

func NewDOCXParserWithOptions(o DOCXOptions) *DOCXParser {
	if o.MaxInputBytes <= 0 {
		o.MaxInputBytes = int64(consts.MaxDocumentSize)
	}
	if o.MaxEntries <= 0 {
		o.MaxEntries = consts.DefaultDOCXMaxEntries
	}
	if o.MaxXMLBytes <= 0 {
		o.MaxXMLBytes = consts.DefaultDOCXMaxXMLSize
	}
	return &DOCXParser{opts: o}
}

func (p *DOCXParser) Parse(ctx context.Context, r io.Reader, mimeType string) (doc *models.ParsedDocument, err error) {
	if mimeType != consts.DocxMIME {
		return nil, fmt.Errorf("%w: docx parser received %q", errs.ErrUnsupportedFileType, mimeType)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	data, err := readAllLimited(r, p.opts.MaxInputBytes)
	if err != nil {
		return nil, err
	}

	defer func() {
		if rec := recover(); rec != nil {
			doc, err = nil, fmt.Errorf("%w: docx parser panic: %v", errs.ErrDocumentCorrupt, rec)
		}
	}()

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%w: not a readable zip archive: %w", errs.ErrDocumentCorrupt, err)
	}
	if len(zr.File) > p.opts.MaxEntries {
		return nil, fmt.Errorf("%w: archive has %d entries (max %d)", errs.ErrDocumentTooLarge, len(zr.File), p.opts.MaxEntries)
	}
	if !filetype.IsDOCXPackage(zr) {
		return nil, fmt.Errorf("%w: zip archive is not a Word document", errs.ErrUnsupportedFileType)
	}

	x := &docxExtraction{
		ctx:    ctx,
		opts:   p.opts,
		budget: p.opts.MaxXMLBytes,
		files:  make(map[string]*zip.File, len(zr.File)),
	}
	for _, f := range zr.File {
		x.files[strings.ToLower(f.Name)] = f
	}

	mainName := x.mainPartName()
	mainFile := x.files[mainName]
	if mainFile == nil {
		return nil, fmt.Errorf("%w: main document part %q is missing", errs.ErrDocumentCorrupt, mainName)
	}

	styles := x.loadStyles()

	body, err := x.walk(mainFile, styles)
	if err != nil {
		return nil, classifyDOCXError(mainName, err)
	}

	blocks := x.withHeadersFooters(body, styles)
	applyHeadingHeuristics(blocks)

	sections := buildSections(blocks)
	if len(sections) == 0 {
		return nil, fmt.Errorf("%w: document contains no text", errs.ErrDocumentEmpty)
	}
	return models.NewParsedDocument(sections, x.warnings), nil
}

func classifyDOCXError(part string, err error) error {
	switch {
	case errors.Is(err, errs.ErrDocumentTooLarge),
		errors.Is(err, context.Canceled),
		errors.Is(err, context.DeadlineExceeded):
		return err
	default:
		return fmt.Errorf("%w: %s: %v", errs.ErrDocumentCorrupt, part, err)
	}
}

type docxExtraction struct {
	ctx      context.Context
	opts     DOCXOptions
	files    map[string]*zip.File
	budget   int64
	warnings []string
}

type budgetReader struct {
	r      io.Reader
	budget *int64
	limit  int64
}

func (b *budgetReader) Read(p []byte) (int, error) {
	if *b.budget < 0 {
		return 0, errs.ErrDocumentTooLarge
	}
	n, err := b.r.Read(p)
	*b.budget -= int64(n)
	if *b.budget < 0 {
		return n, fmt.Errorf("%w: decompressed XML exceeds %d bytes", errs.ErrDocumentTooLarge, b.limit)
	}
	return n, err
}

func (x *docxExtraction) newDecoder(f *zip.File) (*xml.Decoder, io.Closer, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, nil, err
	}
	dec := xml.NewDecoder(&budgetReader{r: rc, budget: &x.budget, limit: x.opts.MaxXMLBytes})
	return dec, rc, nil
}

func (x *docxExtraction) mainPartName() string {
	const fallback = "word/document.xml"
	f := x.files["_rels/.rels"]
	if f == nil {
		return fallback
	}
	dec, closer, err := x.newDecoder(f)
	if err != nil {
		return fallback
	}
	defer closer.Close()
	for {
		tok, err := dec.Token()
		if err != nil {
			return fallback
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "Relationship" {
			continue
		}
		if strings.HasSuffix(attrVal(se, "Type"), "/officeDocument") {
			target := strings.TrimPrefix(attrVal(se, "Target"), "/")
			if target != "" {
				return strings.ToLower(path.Clean(target))
			}
		}
	}
}

func (x *docxExtraction) partsWithPrefix(prefix string) []*zip.File {
	var names []string
	for name := range x.files {
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".xml") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	out := make([]*zip.File, 0, len(names))
	for _, n := range names {
		out = append(out, x.files[n])
	}
	return out
}

func (x *docxExtraction) withHeadersFooters(body []docxBlock, styles *styleTable) []docxBlock {
	seenBody := make(map[string]bool)
	for i := 0; i < len(body) && i < consts.HeaderDedupeWindow; i++ {
		seenBody[body[i].text] = true
	}
	collect := func(prefix string) []docxBlock {
		var out []docxBlock
		seen := make(map[string]bool)
		for _, f := range x.partsWithPrefix(prefix) {
			blocks, err := x.walk(f, styles)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return out
				}
				x.warnings = append(x.warnings, fmt.Sprintf("skipped %s: %v", f.Name, err))
				continue
			}
			for _, b := range blocks {
				if seen[b.text] || seenBody[b.text] {
					continue
				}
				seen[b.text] = true
				b.kind, b.level, b.styled = kindParagraph, 0, false
				out = append(out, b)
			}
		}
		return out
	}

	var result []docxBlock
	if !x.opts.ExcludeHeaders {
		result = append(result, collect("word/header")...)
	}
	result = append(result, body...)
	if x.opts.IncludeFooters {
		result = append(result, collect("word/footer")...)
	}
	return result
}

type styleInfo struct {
	name    string
	basedOn string
	outline int
}

type styleTable struct {
	styles map[string]styleInfo
}

func (x *docxExtraction) loadStyles() *styleTable {
	st := &styleTable{styles: map[string]styleInfo{}}
	f := x.files["word/styles.xml"]
	if f == nil {
		return st
	}
	dec, closer, err := x.newDecoder(f)
	if err != nil {
		x.warnings = append(x.warnings, "styles.xml unreadable; heading detection uses style IDs only")
		return st
	}
	defer closer.Close()

	var (
		curID string
		cur   *styleInfo
	)
	for n := 0; ; n++ {
		if n&0x3ff == 0 && x.ctx.Err() != nil {
			return st
		}
		tok, err := dec.Token()
		if err == io.EOF {
			return st
		}
		if err != nil {
			x.warnings = append(x.warnings, fmt.Sprintf("styles.xml parse error: %v; heading detection may be incomplete", err))
			return st
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if !isWordNS(t.Name.Space) {
				continue
			}
			switch t.Name.Local {
			case "style":
				cur = nil
				if attrVal(t, "type") == "paragraph" {
					curID = attrVal(t, "styleId")
					cur = &styleInfo{}
				}
			case "name":
				if cur != nil {
					cur.name = attrVal(t, "val")
				}
			case "basedOn":
				if cur != nil {
					cur.basedOn = attrVal(t, "val")
				}
			case "outlineLvl":
				if cur != nil {
					if v, err := strconv.Atoi(attrVal(t, "val")); err == nil && v >= 0 && v <= 8 {
						cur.outline = v + 1
					}
				}
			}
		case xml.EndElement:
			if isWordNS(t.Name.Space) && t.Name.Local == "style" && cur != nil {
				st.styles[curID] = *cur
				cur = nil
			}
		}
	}
}

func (s *styleTable) heading(id string) (level int, weak bool) {
	for depth := 0; depth < 8 && id != ""; depth++ {
		si, known := s.styles[id]
		name := strings.ToLower(si.name)
		if !known {
			name = strings.ToLower(id)
		}
		if lv := headingNameLevel(name); lv > 0 {
			return lv, false
		}
		if name == "title" {
			return 1, true
		}
		if si.outline > 0 {
			return si.outline, false
		}
		if !known {
			break
		}
		id = si.basedOn
	}
	return 0, false
}

func (s *styleTable) isList(id string) bool {
	if id == "" {
		return false
	}
	name := strings.ToLower(id)
	if si, ok := s.styles[id]; ok && si.name != "" {
		name = strings.ToLower(si.name)
	}
	return strings.HasPrefix(name, "list")
}

func headingNameLevel(name string) int {
	rest, ok := strings.CutPrefix(name, "heading")
	if !ok {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(rest))
	if err != nil || n < 1 || n > 9 {
		return 0
	}
	return n
}

type blockKind uint8

const (
	kindParagraph blockKind = iota
	kindHeading
	kindListItem
)

type docxBlock struct {
	kind    blockKind
	level   int
	text    string
	allBold bool
	styled  bool
}

type paraState struct {
	style    string
	numbered bool
	outline  int

	inPPr, inNumPr, inRPr, inT, curBold bool
	boldRuns, plainRuns                 int
	text                                strings.Builder
}

type (
	cellState  struct{ blocks []docxBlock }
	rowState   struct{ cells [][]docxBlock }
	tableState struct{ rows [][][]docxBlock }
)

func (x *docxExtraction) walk(f *zip.File, styles *styleTable) ([]docxBlock, error) {
	dec, closer, err := x.newDecoder(f)
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	var (
		out    []docxBlock
		paras  []*paraState
		tables []*tableState
		rows   []*rowState
		cells  []*cellState
	)
	emit := func(bs ...docxBlock) {
		if len(cells) > 0 {
			c := cells[len(cells)-1]
			c.blocks = append(c.blocks, bs...)
			return
		}
		out = append(out, bs...)
	}
	top := func() *paraState {
		if len(paras) == 0 {
			return nil
		}
		return paras[len(paras)-1]
	}

	for n := 0; ; n++ {
		if n&0x3ff == 0 {
			if err := x.ctx.Err(); err != nil {
				return nil, err
			}
		}
		tok, err := dec.Token()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Space == nsMC && t.Name.Local == "Fallback" {
				if err := dec.Skip(); err != nil {
					return nil, err
				}
				continue
			}
			if !isWordNS(t.Name.Space) {
				continue
			}
			p := top()
			switch t.Name.Local {
			case "p":
				paras = append(paras, &paraState{})
			case "pPr":
				if p != nil {
					p.inPPr = true
				}
			case "pStyle":
				if p != nil && p.inPPr {
					p.style = attrVal(t, "val")
				}
			case "numPr":
				if p != nil && p.inPPr {
					p.inNumPr = true
				}
			case "numId":
				if p != nil && p.inNumPr && attrVal(t, "val") != "0" {
					p.numbered = true
				}
			case "outlineLvl":
				if p != nil && p.inPPr {
					if v, err := strconv.Atoi(attrVal(t, "val")); err == nil && v >= 0 && v <= 8 {
						p.outline = v + 1
					}
				}
			case "rPr":
				if p != nil {
					p.inRPr = true
				}
			case "r":
				if p != nil {
					p.curBold = false
				}
			case "b":
				if p != nil && p.inRPr && !p.inPPr {
					switch attrVal(t, "val") {
					case "0", "false", "off":
						p.curBold = false
					default:
						p.curBold = true
					}
				}
			case "t":
				if p != nil {
					p.inT = true
				}
			case "tab", "ptab":
				if p != nil && !p.inPPr {
					p.text.WriteByte('\t')
				}
			case "br", "cr":
				if p != nil && !p.inPPr {
					if bt := attrVal(t, "type"); bt != "page" && bt != "column" {
						p.text.WriteByte('\n')
					}
				}
			case "noBreakHyphen":
				if p != nil {
					p.text.WriteByte('-')
				}
			case "tbl":
				tables = append(tables, &tableState{})
			case "tr":
				rows = append(rows, &rowState{})
			case "tc":
				cells = append(cells, &cellState{})
			}

		case xml.CharData:
			if p := top(); p != nil && p.inT {
				p.text.Write(t)
				if strings.TrimSpace(string(t)) != "" {
					if p.curBold {
						p.boldRuns++
					} else {
						p.plainRuns++
					}
				}
			}

		case xml.EndElement:
			if !isWordNS(t.Name.Space) {
				continue
			}
			p := top()
			switch t.Name.Local {
			case "t":
				if p != nil {
					p.inT = false
				}
			case "pPr":
				if p != nil {
					p.inPPr = false
				}
			case "numPr":
				if p != nil {
					p.inNumPr = false
				}
			case "rPr":
				if p != nil {
					p.inRPr = false
				}
			case "p":
				if len(paras) > 0 {
					done := paras[len(paras)-1]
					paras = paras[:len(paras)-1]
					if b, ok := finishParagraph(done, styles); ok {
						emit(b)
					}
				}
			case "tc":
				if len(cells) > 0 {
					c := cells[len(cells)-1]
					cells = cells[:len(cells)-1]
					if len(rows) > 0 {
						r := rows[len(rows)-1]
						r.cells = append(r.cells, c.blocks)
					}
				}
			case "tr":
				if len(rows) > 0 {
					r := rows[len(rows)-1]
					rows = rows[:len(rows)-1]
					if len(tables) > 0 {
						tb := tables[len(tables)-1]
						tb.rows = append(tb.rows, r.cells)
					}
				}
			case "tbl":
				if len(tables) > 0 {
					tb := tables[len(tables)-1]
					tables = tables[:len(tables)-1]
					emit(renderTable(tb)...)
				}
			}
		}
	}
}

func finishParagraph(p *paraState, styles *styleTable) (docxBlock, bool) {
	lines := cleanLines(p.text.String())
	if len(lines) == 0 {
		return docxBlock{}, false
	}
	b := docxBlock{
		text:    strings.Join(lines, "\n"),
		allBold: p.boldRuns > 0 && p.plainRuns == 0,
	}

	level, weak := 0, false
	if p.outline > 0 {
		level = p.outline
	} else {
		level, weak = styles.heading(p.style)
	}

	oneLine := strings.Join(lines, " ")
	switch {
	case level > 0 && utf8.RuneCountInString(oneLine) <= consts.MaxHeadingRunes:
		b.kind, b.level, b.text, b.styled = kindHeading, level, oneLine, !weak
	case p.numbered || styles.isList(p.style):
		b.kind = kindListItem
	}
	return b, true
}

func cleanLines(s string) []string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\u00a0' || r == '\t' || r == '\u2007' || r == '\u202f':
			return ' '
		case r == '\u200b' || r == '\ufeff' || r == '\u00ad':
			return -1
		case r != '\n' && unicode.IsControl(r):
			return -1
		}
		return r
	}, s)
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if f := strings.Join(strings.Fields(line), " "); f != "" {
			out = append(out, f)
		}
	}
	return out
}

func renderTable(t *tableState) []docxBlock {
	var out []docxBlock
	for _, row := range t.rows {
		var cells [][]docxBlock
		for _, c := range row {
			if len(c) > 0 {
				cells = append(cells, c)
			}
		}
		switch {
		case len(cells) == 0:
			continue
		case rowIsCompact(cells):
			if len(cells) == 1 {
				out = append(out, cells[0][0])
				continue
			}
			parts := make([]string, 0, len(cells))
			for _, c := range cells {
				parts = append(parts, strings.ReplaceAll(c[0].text, "\n", " "))
			}
			out = append(out, docxBlock{kind: kindParagraph, text: strings.Join(parts, " | ")})
		default:
			for _, c := range cells {
				out = append(out, c...)
			}
		}
	}
	return out
}

func rowIsCompact(cells [][]docxBlock) bool {
	for _, c := range cells {
		if len(c) > 1 || c[0].kind == kindHeading {
			return false
		}
	}
	return true
}

var resumeSectionNames = map[string]bool{
	"summary": true, "professional summary": true, "profile": true,
	"objective": true, "career objective": true, "about me": true,
	"experience": true, "work experience": true, "professional experience": true,
	"employment": true, "employment history": true, "work history": true,
	"education": true, "skills": true, "technical skills": true, "key skills": true,
	"projects": true, "certifications": true, "certificates": true,
	"awards": true, "achievements": true, "publications": true,
	"languages": true, "interests": true, "references": true,
	"volunteer experience": true, "contact": true,
}

func applyHeadingHeuristics(blocks []docxBlock) {
	for _, b := range blocks {
		if b.kind == kindHeading && b.styled {
			return
		}
	}
	for i := range blocks {
		if blocks[i].kind == kindParagraph && looksLikeSectionHeading(blocks[i]) {
			blocks[i].kind, blocks[i].level = kindHeading, 1
		}
	}
}

func looksLikeSectionHeading(b docxBlock) bool {
	if strings.Contains(b.text, "\n") {
		return false
	}
	text := strings.TrimSpace(strings.TrimRight(b.text, ":"))
	n := utf8.RuneCountInString(text)
	if n < 3 || n > 50 {
		return false
	}
	if resumeSectionNames[strings.ToLower(text)] {
		return true
	}
	return b.allBold && isAllCaps(text) && !strings.ContainsAny(text, ".!?")
}

func isAllCaps(s string) bool {
	hasUpper := false
	for _, r := range s {
		if unicode.IsLower(r) {
			return false
		}
		if unicode.IsUpper(r) {
			hasUpper = true
		}
	}
	return hasUpper
}

type sectionDraft struct {
	title string
	level int
	lines []string
	body  int
}

type headingFrame struct {
	level int
	text  string
}

func buildSections(blocks []docxBlock) []models.Section {
	var (
		out   []models.Section
		stack []headingFrame
		cur   = &sectionDraft{}
	)
	flush := func() {
		if len(cur.lines) > 0 {
			out = append(out, models.Section{Title: cur.title, Content: strings.Join(cur.lines, "\n")})
		}
	}

	for _, b := range blocks {
		if b.kind != kindHeading {
			cur.lines = append(cur.lines, renderBlockLines(b)...)
			cur.body++
			continue
		}

		for len(stack) > 0 && stack[len(stack)-1].level >= b.level {
			stack = stack[:len(stack)-1]
		}
		stack = append(stack, headingFrame{level: b.level, text: b.text})

		var carry []string
		if cur.body == 0 && cur.level > 0 && b.level > cur.level {
			carry = append(carry, cur.lines...)
		} else {
			flush()
		}
		cur = &sectionDraft{
			title: headingPath(stack),
			level: b.level,
			lines: append(carry, b.text),
		}
	}
	flush()
	return out
}

func renderBlockLines(b docxBlock) []string {
	lines := strings.Split(b.text, "\n")
	if b.kind == kindListItem {
		lines[0] = "- " + lines[0]
	}
	return lines
}

func headingPath(stack []headingFrame) string {
	parts := make([]string, len(stack))
	for i, f := range stack {
		parts[i] = f.text
	}
	title := strings.Join(parts, " > ")
	if r := []rune(title); len(r) > consts.MaxTitleRunes {
		title = string(r[:consts.MaxTitleRunes])
	}
	return title
}

func isWordNS(space string) bool {
	return space == nsWordTransitional || space == nsWordStrict
}

func attrVal(se xml.StartElement, local string) string {
	for _, a := range se.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}
