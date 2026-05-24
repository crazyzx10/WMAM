package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

const encryptedFieldVersion = "v1"

func EncryptField(key []byte, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if len(key) != 32 {
		return "", errors.New("field encryption key must be 32 bytes")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return strings.Join([]string{
		encryptedFieldVersion,
		base64.RawStdEncoding.EncodeToString(nonce),
		base64.RawStdEncoding.EncodeToString(ciphertext),
	}, ":"), nil
}

func DecryptField(key []byte, value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if len(key) != 32 {
		return "", errors.New("field encryption key must be 32 bytes")
	}

	parts := strings.Split(value, ":")
	if len(parts) != 3 || parts[0] != encryptedFieldVersion {
		return "", fmt.Errorf("unsupported encrypted field format")
	}

	nonce, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}
	ciphertext, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
