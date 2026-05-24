package security

import (
	"crypto/rand"
	"encoding/base32"
	"strings"
)

func RandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	return b, err
}

func NewRecoveryCode() (string, error) {
	b, err := RandomBytes(15)
	if err != nil {
		return "", err
	}
	raw := strings.TrimRight(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), "=")
	chunks := []string{"WMAM"}
	for len(raw) > 0 {
		end := 4
		if len(raw) < end {
			end = len(raw)
		}
		chunks = append(chunks, raw[:end])
		raw = raw[end:]
	}
	return strings.Join(chunks, "-"), nil
}
