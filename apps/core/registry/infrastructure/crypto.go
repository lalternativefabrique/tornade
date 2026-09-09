package infrastructure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// Cipher seals an app's keys with AES-256-GCM, so a copy of the database is
// not a copy of every application's credentials.
type Cipher struct {
	aead cipher.AEAD
}

// NewCipherFromBase64 builds a Cipher from a base64-encoded 32-byte key.
func NewCipherFromBase64(b64Key string) (*Cipher, error) {
	if b64Key == "" {
		return nil, errors.New("registry: encryption key is empty")
	}
	key, err := base64.StdEncoding.DecodeString(b64Key)
	if err != nil {
		return nil, fmt.Errorf("registry: decode encryption key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("registry: encryption key must be 32 bytes (got %d)", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt seals plaintext as nonce || ciphertext, one blob to store.
func (c *Cipher) Encrypt(plaintext string) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return append(nonce, c.aead.Seal(nil, nonce, []byte(plaintext), nil)...), nil
}

// Decrypt opens a blob produced by Encrypt.
func (c *Cipher) Decrypt(blob []byte) (string, error) {
	ns := c.aead.NonceSize()
	if len(blob) < ns {
		return "", errors.New("registry: ciphertext too short")
	}
	plaintext, err := c.aead.Open(nil, blob[:ns], blob[ns:], nil)
	if err != nil {
		return "", fmt.Errorf("registry: decrypt: %w", err)
	}
	return string(plaintext), nil
}
