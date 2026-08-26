package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
)

// EncryptMetadata encrypts small sensitive blobs with AES-256-GCM using key material from envVar.
// When envVar is empty or unset, returns plaintext prefixed with "plain:" for round-trip compatibility.
func EncryptMetadata(envVar string, plaintext []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return plaintext, nil
	}
	key, ok := loadKey(envVar)
	if !ok {
		return append([]byte("plain:"), plaintext...), nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, plaintext, nil)
	out := append([]byte("enc:"), sealed...)
	return out, nil
}

// DecryptMetadata reverses EncryptMetadata.
func DecryptMetadata(envVar string, stored []byte) ([]byte, error) {
	if len(stored) == 0 {
		return stored, nil
	}
	if len(stored) >= 6 && string(stored[:6]) == "plain:" {
		return stored[6:], nil
	}
	if len(stored) < 4 || string(stored[:4]) != "enc:" {
		return nil, fmt.Errorf("unknown metadata encoding")
	}
	payload := stored[4:]
	key, ok := loadKey(envVar)
	if !ok {
		return nil, fmt.Errorf("metadata is encrypted but %q is not set", envVar)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	if len(payload) < gcm.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := payload[:gcm.NonceSize()], payload[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt metadata: %w", err)
	}
	return plain, nil
}

func loadKey(envVar string) ([]byte, bool) {
	if envVar == "" {
		return nil, false
	}
	raw := os.Getenv(envVar)
	if raw == "" {
		return nil, false
	}
	sum := sha256.Sum256([]byte(raw))
	return sum[:], true
}

// EncodeMetadataBase64 stores encrypted bytes as base64 text for SQLite columns.
func EncodeMetadataBase64(envVar string, plaintext []byte) (string, error) {
	enc, err := EncryptMetadata(envVar, plaintext)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(enc), nil
}

// DecodeMetadataBase64 decodes base64 metadata blobs.
func DecodeMetadataBase64(envVar, encoded string) ([]byte, error) {
	if encoded == "" {
		return nil, nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode metadata base64: %w", err)
	}
	return DecryptMetadata(envVar, raw)
}
