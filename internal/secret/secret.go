// Package secret encrypts credentials stored in the database (agent tokens,
// SFTP passwords and keys) with a per-installation key kept next to it.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Box seals and opens values with AES-256-GCM.
type Box struct{ aead cipher.AEAD }

// Open loads <dir>/secret.key, creating it (0600) on first use.
func Open(dir string) (*Box, error) {
	path := filepath.Join(dir, "secret.key")
	key, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, key, 0o600); err != nil {
			return nil, fmt.Errorf("secret key: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("secret key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("secret key %s: invalid length %d", path, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// SealJSON encrypts v as JSON. Output: nonce || ciphertext.
func (b *Box) SealJSON(v any) ([]byte, error) {
	plain, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return b.aead.Seal(nonce, nonce, plain, nil), nil
}

// OpenJSON decrypts data produced by SealJSON into v. Empty data is a no-op.
func (b *Box) OpenJSON(data []byte, v any) error {
	if len(data) == 0 {
		return nil
	}
	n := b.aead.NonceSize()
	if len(data) < n {
		return errors.New("secret: ciphertext too short")
	}
	plain, err := b.aead.Open(nil, data[:n], data[n:], nil)
	if err != nil {
		return fmt.Errorf("secret: %w (clé secret.key changée ?)", err)
	}
	return json.Unmarshal(plain, v)
}
