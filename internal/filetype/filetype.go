// Package filetype is the single place that decides which document format an
// upload is. The upload gate (internal/document) and the parsers
// (internal/ingestion) both depend on it, so they cannot disagree.
package filetype

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"strings"

	"github.com/mtariq99/retrievion/internal/consts"
)

const (
	pdfMagic         = "%PDF-"
	zipMagic         = "PK\x03\x04"
	contentTypesXML  = "[Content_Types].xml"
	docxMainPartType = "wordprocessingml.document.main+xml"
	maxContentTypes  = 1 << 20
)

func Detect(r io.ReaderAt, size int64) (string, error) {
	var head [8]byte
	n, err := r.ReadAt(head[:], 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	switch {
	case bytes.HasPrefix(head[:n], []byte(pdfMagic)):
		return consts.PDF, nil
	case bytes.HasPrefix(head[:n], []byte(zipMagic)):
		zr, err := zip.NewReader(r, size)
		if err != nil {
			return "", nil
		}
		if IsDOCXPackage(zr) {
			return consts.DOCX, nil
		}
	}
	return "", nil
}

func IsDOCXPackage(zr *zip.Reader) bool {
	for _, f := range zr.File {
		if !strings.EqualFold(f.Name, contentTypesXML) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return false
		}
		defer rc.Close()
		data, err := io.ReadAll(io.LimitReader(rc, maxContentTypes+1))
		if err != nil || len(data) > maxContentTypes {
			return false
		}
		return bytes.Contains(data, []byte(docxMainPartType))
	}
	return false
}
