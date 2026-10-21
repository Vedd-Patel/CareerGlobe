package resume

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestVaultRoundTripAndBinding(t *testing.T) {
	dir := t.TempDir()
	v, err := NewVault(dir, bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	name, err := v.Put(42, []byte("%PDF-1.7 secret resume"))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, name))
	if bytes.Contains(raw, []byte("secret")) {
		t.Fatal("stored in plain text")
	}
	if info, _ := os.Stat(filepath.Join(dir, name)); info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", info.Mode())
	}
	got, err := v.Get(42, name)
	if err != nil || string(got) != "%PDF-1.7 secret resume" {
		t.Fatalf("Get = %q, %v", got, err)
	}
	if _, err := v.Get(43, name); err == nil {
		t.Fatal("another user's ID decrypted the file")
	}
	if _, err := v.Get(42, "../"+name); err == nil {
		t.Fatal("path traversal accepted")
	}
	other, _ := NewVault(dir, bytes.Repeat([]byte{8}, 32))
	if _, err := other.Get(42, name); err == nil {
		t.Fatal("wrong key decrypted the file")
	}
	if err := v.Delete(name); err != nil || v.Delete(name) != nil {
		t.Fatal("delete")
	}
}

func TestKindAndText(t *testing.T) {
	if Kind([]byte("%PDF-1.4 ...")) != "pdf" || Kind([]byte("hello")) != "" {
		t.Fatal("kind")
	}
	if _, err := Text([]byte("not a resume")); err != ErrUnsupported {
		t.Fatalf("err = %v", err)
	}
	if _, err := Text(make([]byte, MaxUpload+1)); err != ErrTooLarge {
		t.Fatalf("err = %v", err)
	}
	doc := docx(t, "<w:document xmlns:w=\"w\"><w:body><w:p><w:r><w:t>Senior Engineer</w:t></w:r></w:p>"+
		"<w:p><w:r><w:t>Skills: Java, Kafka, AWS and a lot more words to pass the twenty word minimum for a real resume text here today ok</w:t></w:r></w:p></w:body></w:document>")
	text, err := Text(doc)
	if err != nil || text[:15] != "Senior Engineer" {
		t.Fatalf("docx text = %q, %v", text, err)
	}
}
