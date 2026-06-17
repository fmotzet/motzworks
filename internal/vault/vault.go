// Package vault seals and opens credential secrets using NaCl secretbox with a
// symmetric key. Stored secrets are base64(nonce||ciphertext); the key itself
// lives outside the database (env var / KMS), never in it.
package vault

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/crypto/nacl/secretbox"
)

const (
	keySize   = 32
	nonceSize = 24
)

// Vault encrypts and decrypts secrets with a single symmetric key.
type Vault struct {
	key [keySize]byte
}

// GenerateKey returns a fresh base64-encoded 32-byte key.
func GenerateKey() (string, error) {
	var k [keySize]byte
	if _, err := io.ReadFull(rand.Reader, k[:]); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(k[:]), nil
}

// New builds a Vault from a base64-encoded 32-byte key.
func New(b64key string) (*Vault, error) {
	raw, err := base64.StdEncoding.DecodeString(b64key)
	if err != nil {
		return nil, fmt.Errorf("decode vault key: %w", err)
	}
	if len(raw) != keySize {
		return nil, fmt.Errorf("vault key must be %d bytes, got %d", keySize, len(raw))
	}
	v := &Vault{}
	copy(v.key[:], raw)
	return v, nil
}

// FromEnv builds a Vault from the base64 key stored in the named env var.
func FromEnv(envName string) (*Vault, error) {
	b64 := os.Getenv(envName)
	if b64 == "" {
		return nil, fmt.Errorf("vault key env %q is not set", envName)
	}
	return New(b64)
}

// Seal encrypts plaintext and returns base64(nonce||ciphertext).
func (v *Vault) Seal(plaintext []byte) (string, error) {
	var nonce [nonceSize]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return "", err
	}
	sealed := secretbox.Seal(nonce[:], plaintext, &nonce, &v.key)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Open decrypts a value produced by Seal.
func (v *Vault) Open(b64 string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("decode sealed value: %w", err)
	}
	if len(data) < nonceSize {
		return nil, errors.New("sealed value too short")
	}
	var nonce [nonceSize]byte
	copy(nonce[:], data[:nonceSize])
	out, ok := secretbox.Open(nil, data[nonceSize:], &nonce, &v.key)
	if !ok {
		return nil, errors.New("decryption failed (wrong key or corrupt data)")
	}
	return out, nil
}
