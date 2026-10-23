package filetype

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"strings"

	"github.com/mtariq99/retrievion/internal/consts"
)

func Detect(r io.ReaderAt, size int64) (string, error) {
	var head [8]byte
	n, err := r.ReadAt(head[:], 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	switch {
	case bytes.HasPrefix(head[:n], []byte(consts.PdfMagic)):
		return consts.PDFMIME, nil
	case bytes.HasPrefix(head[:n], []byte(consts.ZipMagic)):
		zr, err := zip.NewReader(r, size)
		if err != nil {
			return "", nil
		}
		if IsDOCXPackage(zr) {
			return consts.DOCXMIME, nil
		}
	}
	return "", nil
}

func IsDOCXPackage(zr *zip.Reader) bool {
	for _, f := range zr.File {
		if !strings.EqualFold(f.Name, consts.ContentTypesXML) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return false
		}
		defer rc.Close()
		data, err := io.ReadAll(io.LimitReader(rc, consts.MaxContentTypes+1))
		if err != nil || len(data) > consts.MaxContentTypes {
			return false
		}
		return bytes.Contains(data, []byte(consts.DocxMainPartType))
	}
	return false
}
