package security

import (
	"os"
	"path/filepath"
)

func LoadOrCreateFieldKey(dataDir string) ([]byte, error) {
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return nil, err
	}

	keyPath := filepath.Join(dataDir, "secret.key")
	key, err := os.ReadFile(keyPath)
	if err == nil {
		if len(key) == 32 {
			return key, nil
		}
		return nil, os.ErrInvalid
	}
	if !os.IsNotExist(err) {
		return nil, err
	}

	key, err = RandomBytes(32)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, key, 0600); err != nil {
		return nil, err
	}
	return key, nil
}
