// Package resume stores uploaded resumes encrypted, extracts their text and
// parses a draft profile for the user to confirm (spec: Parsing the resume).
package resume

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var magic = []byte("CGR1") // file format version

// Vault keeps resume files AES-256-GCM encrypted under random names in one
// directory outside the web root. The key comes from RESUME_ENCRYPTION_KEY.
type Vault struct {
	dir  string
	aead cipher.AEAD
}

// NewVault checks the key and creates the directory (0700).
func NewVault(dir string, key []byte) (*Vault, error) {
	if len(key) != 32 {
		return nil, errors.New("resume: encryption key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Vault{dir: dir, aead: aead}, nil
}

// Put encrypts data to a new random file and returns its name (not a path).
// The user ID is bound into the ciphertext, so a file cannot be swapped between users.
func (v *Vault) Put(userID int64, data []byte) (string, error) {
	name := make([]byte, 16)
	if _, err := rand.Read(name); err != nil {
		return "", err
	}
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := append(append(append([]byte{}, magic...), nonce...), v.aead.Seal(nil, nonce, data, ad(userID))...)
	file := hex.EncodeToString(name) + ".bin"
	if err := os.WriteFile(filepath.Join(v.dir, file), out, 0o600); err != nil {
		return "", err
	}
	return file, nil
}

// Get decrypts a stored file.
func (v *Vault) Get(userID int64, file string) ([]byte, error) {
	if filepath.Base(file) != file {
		return nil, errors.New("resume: bad file name")
	}
	raw, err := os.ReadFile(filepath.Join(v.dir, file))
	if err != nil {
		return nil, err
	}
	n := len(magic) + v.aead.NonceSize()
	if len(raw) < n || string(raw[:len(magic)]) != string(magic) {
		return nil, errors.New("resume: not a resume file")
	}
	return v.aead.Open(nil, raw[len(magic):n], raw[n:], ad(userID))
}

// Delete removes a stored file (replaced resumes, account deletion).
func (v *Vault) Delete(file string) error {
	if file == "" || filepath.Base(file) != file {
		return nil
	}
	err := os.Remove(filepath.Join(v.dir, file))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func ad(userID int64) []byte { return []byte(fmt.Sprintf("careerglobe-resume/%d", userID)) }
