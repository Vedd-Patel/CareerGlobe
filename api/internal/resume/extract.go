package resume

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
)

// MaxUpload is the largest resume accepted (spec: 5 MB).
const MaxUpload = 5 << 20

const maxDocxXML = 20 << 20 // unzipped document.xml limit (zip bombs)

var (
	ErrTooLarge    = errors.New("resume: file is larger than 5 MB")
	ErrUnsupported = errors.New("resume: only PDF and DOCX files are accepted")
	ErrNoText      = errors.New("resume: no text found (a scanned PDF?)")
)

// Kind sniffs the file type from its bytes, not its name.
func Kind(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte("%PDF-")):
		return "pdf"
	case bytes.HasPrefix(data, []byte("PK\x03\x04")):
		if zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data))); err == nil {
			for _, f := range zr.File {
				if f.Name == "word/document.xml" {
					return "docx"
				}
			}
		}
	}
	return ""
}

// ContentType for a sniffed kind.
func ContentType(kind string) string {
	if kind == "pdf" {
		return "application/pdf"
	}
	return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
}

// Text extracts plain text from a PDF or DOCX resume.
func Text(data []byte) (string, error) {
	if len(data) > MaxUpload {
		return "", ErrTooLarge
	}
	var (
		text string
		err  error
	)
	switch Kind(data) {
	case "pdf":
		text, err = pdfText(data)
	case "docx":
		text, err = docxText(data)
	default:
		return "", ErrUnsupported
	}
	if err != nil {
		return "", err
	}
	if len(strings.Fields(text)) < 20 {
		return "", ErrNoText
	}
	return text, nil
}

func pdfText(data []byte) (out string, err error) {
	defer func() { // the PDF library panics on some malformed files
		if recover() != nil {
			out, err = "", ErrNoText
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", ErrNoText
	}
	var b strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		rows, err := p.GetTextByRow()
		if err != nil {
			continue
		}
		for _, row := range rows {
			for j, word := range row.Content {
				if j > 0 {
					b.WriteByte(' ')
				}
				b.WriteString(word.S)
			}
			b.WriteByte('\n')
		}
	}
	return b.String(), nil
}

func docxText(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", ErrUnsupported
	}
	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", ErrUnsupported
		}
		defer rc.Close()
		return docxXMLText(io.LimitReader(rc, maxDocxXML))
	}
	return "", ErrUnsupported
}

// docxXMLText keeps the text runs (w:t), with a line per paragraph (w:p) and tabs/breaks as spaces.
func docxXMLText(r io.Reader) (string, error) {
	dec := xml.NewDecoder(r)
	var b strings.Builder
	inText := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return b.String(), nil
		}
		if err != nil {
			return "", ErrUnsupported
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "t":
				inText = true
			case "tab", "br":
				b.WriteByte(' ')
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "p":
				b.WriteByte('\n')
			}
		case xml.CharData:
			if inText {
				b.Write(t)
			}
		}
	}
}
