package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

const sessionTokenBytes = 32

type CredentialCipher struct {
	aead       cipher.AEAD
	keyVersion int
}

func NewCredentialCipher(secret string, keyVersion int) (*CredentialCipher, error) {
	key, err := decodeKey(secret)
	if err != nil {
		return nil, err
	}
	if keyVersion < 1 {
		return nil, errors.New("credential key version must be positive")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &CredentialCipher{aead: aead, keyVersion: keyVersion}, nil
}

func (c *CredentialCipher) Encrypt(plaintext string, aad string) (ciphertext, nonce []byte, keyVersion int, err error) {
	if c == nil {
		return nil, nil, 0, errors.New("credential cipher is not configured")
	}
	nonce = make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, 0, err
	}
	return c.aead.Seal(nil, nonce, []byte(plaintext), []byte(aad)), nonce, c.keyVersion, nil
}

func (c *CredentialCipher) Decrypt(ciphertext, nonce []byte, keyVersion int, aad string) (string, error) {
	if c == nil {
		return "", errors.New("credential cipher is not configured")
	}
	if keyVersion != c.keyVersion {
		return "", fmt.Errorf("unsupported credential key version %d", keyVersion)
	}
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, []byte(aad))
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func GenerateSessionToken() (string, []byte, error) {
	raw := make([]byte, sessionTokenBytes)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", nil, err
	}
	return base64.RawURLEncoding.EncodeToString(raw), SessionTokenHash(raw), nil
}

func SessionTokenHashFromCookie(value string) ([]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	if len(raw) != sessionTokenBytes {
		return nil, errors.New("invalid session token length")
	}
	return SessionTokenHash(raw), nil
}

func SessionTokenHash(raw []byte) []byte {
	hash := sha256.Sum256(raw)
	return hash[:]
}

func TokenLast4(token string) string {
	token = strings.TrimSpace(token)
	if len(token) <= 4 {
		return token
	}
	return token[len(token)-4:]
}

func decodeKey(secret string) ([]byte, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, errors.New("credential key is empty")
	}
	if decoded, err := base64.StdEncoding.DecodeString(secret); err == nil && validAESKeySize(len(decoded)) {
		return decoded, nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(secret); err == nil && validAESKeySize(len(decoded)) {
		return decoded, nil
	}
	if decoded, err := base64.RawURLEncoding.DecodeString(secret); err == nil && validAESKeySize(len(decoded)) {
		return decoded, nil
	}
	if decoded, err := hex.DecodeString(secret); err == nil && validAESKeySize(len(decoded)) {
		return decoded, nil
	}
	if validAESKeySize(len(secret)) {
		return []byte(secret), nil
	}
	return nil, errors.New("credential key must decode to 16, 24, or 32 bytes")
}

func validAESKeySize(size int) bool {
	return size == 16 || size == 24 || size == 32
}
