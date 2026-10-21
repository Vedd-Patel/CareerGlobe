package resume

import (
	"archive/zip"
	"bytes"
	"testing"
)

func docx(t *testing.T, documentXML string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte(documentXML))
	zw.Close()
	return buf.Bytes()
}
