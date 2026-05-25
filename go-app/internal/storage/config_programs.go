package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"go-app/internal/security"
)

type MySQLConfig struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Database    string `json:"database"`
	Username    string `json:"username"`
	Password    string `json:"password,omitempty"`
	PasswordSet bool   `json:"passwordSet"`
}

type MiniProgram struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	AppID           string `json:"appId"`
	AppIDMasked     string `json:"appIdMasked"`
	AppSecret       string `json:"-"`
	AppSecretSet    bool   `json:"appSecretSet"`
	Enabled         bool   `json:"enabled"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
	encryptedSecret string
}

func GetConfigValue(db *sql.DB, key string) (string, bool, error) {
	var value string
	err := db.QueryRow("SELECT value FROM system_config WHERE key = ?", key).Scan(&value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return value, true, nil
}

func SetConfigValue(db *sql.DB, key, value string, encrypted bool) error {
	encryptedValue := 0
	if encrypted {
		encryptedValue = 1
	}
	_, err := db.Exec(`
INSERT INTO system_config (key, value, encrypted, updated_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(key) DO UPDATE SET
    value = excluded.value,
    encrypted = excluded.encrypted,
    updated_at = CURRENT_TIMESTAMP
`, key, value, encryptedValue)
	return err
}

func GetMySQLConfig(db *sql.DB, fieldKey []byte, includePassword bool) (MySQLConfig, error) {
	cfg := MySQLConfig{Port: 3306}

	if value, ok, err := GetConfigValue(db, "mysql.host"); err != nil {
		return cfg, err
	} else if ok {
		cfg.Host = value
	}
	if value, ok, err := GetConfigValue(db, "mysql.port"); err != nil {
		return cfg, err
	} else if ok {
		_, _ = fmt.Sscanf(value, "%d", &cfg.Port)
	}
	if value, ok, err := GetConfigValue(db, "mysql.database"); err != nil {
		return cfg, err
	} else if ok {
		cfg.Database = value
	}
	if value, ok, err := GetConfigValue(db, "mysql.username"); err != nil {
		return cfg, err
	} else if ok {
		cfg.Username = value
	}
	if value, ok, err := GetConfigValue(db, "mysql.password"); err != nil {
		return cfg, err
	} else if ok && value != "" {
		cfg.PasswordSet = true
		if includePassword {
			password, err := security.DecryptField(fieldKey, value)
			if err != nil {
				return cfg, err
			}
			cfg.Password = password
		}
	}

	return cfg, nil
}

func SaveMySQLConfig(db *sql.DB, fieldKey []byte, next MySQLConfig) error {
	current, err := GetMySQLConfig(db, fieldKey, true)
	if err != nil {
		return err
	}
	if strings.TrimSpace(next.Password) == "" {
		next.Password = current.Password
	}
	if next.Port == 0 {
		next.Port = 3306
	}

	if current.Host != "" || current.Database != "" || current.Username != "" || current.Password != "" {
		raw, err := json.Marshal(current)
		if err != nil {
			return err
		}
		encrypted, err := security.EncryptField(fieldKey, string(raw))
		if err != nil {
			return err
		}
		if err := SetConfigValue(db, "mysql.last_good_config", encrypted, true); err != nil {
			return err
		}
	}

	passwordEncrypted, err := security.EncryptField(fieldKey, next.Password)
	if err != nil {
		return err
	}

	if err := SetConfigValue(db, "mysql.host", strings.TrimSpace(next.Host), false); err != nil {
		return err
	}
	if err := SetConfigValue(db, "mysql.port", fmt.Sprintf("%d", next.Port), false); err != nil {
		return err
	}
	if err := SetConfigValue(db, "mysql.database", strings.TrimSpace(next.Database), false); err != nil {
		return err
	}
	if err := SetConfigValue(db, "mysql.username", strings.TrimSpace(next.Username), false); err != nil {
		return err
	}
	return SetConfigValue(db, "mysql.password", passwordEncrypted, true)
}

func RestoreLastGoodMySQLConfig(db *sql.DB, fieldKey []byte) (MySQLConfig, error) {
	encrypted, ok, err := GetConfigValue(db, "mysql.last_good_config")
	if err != nil {
		return MySQLConfig{}, err
	}
	if !ok || encrypted == "" {
		return MySQLConfig{}, errors.New("no last good mysql config")
	}

	raw, err := security.DecryptField(fieldKey, encrypted)
	if err != nil {
		return MySQLConfig{}, err
	}

	var cfg MySQLConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return MySQLConfig{}, err
	}
	return cfg, SaveMySQLConfig(db, fieldKey, cfg)
}

func HasLastGoodMySQLConfig(db *sql.DB) (bool, error) {
	encrypted, ok, err := GetConfigValue(db, "mysql.last_good_config")
	if err != nil {
		return false, err
	}
	return ok && strings.TrimSpace(encrypted) != "", nil
}

func ListMiniPrograms(db *sql.DB, includeSecret bool, fieldKey []byte) ([]MiniProgram, error) {
	rows, err := db.Query(`
SELECT id, name, app_id, app_secret_encrypted, enabled, created_at, updated_at
FROM mini_programs
ORDER BY created_at DESC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	programs := make([]MiniProgram, 0)
	for rows.Next() {
		var program MiniProgram
		var enabled int
		if err := rows.Scan(&program.ID, &program.Name, &program.AppID, &program.encryptedSecret, &enabled, &program.CreatedAt, &program.UpdatedAt); err != nil {
			return nil, err
		}
		program.Enabled = enabled == 1
		program.AppIDMasked = MaskAppID(program.AppID)
		program.AppSecretSet = program.encryptedSecret != ""
		if includeSecret && program.encryptedSecret != "" {
			secret, err := security.DecryptField(fieldKey, program.encryptedSecret)
			if err != nil {
				return nil, err
			}
			program.AppSecret = secret
		}
		programs = append(programs, program)
	}
	return programs, rows.Err()
}

func ListEnabledMiniProgramsWithSecret(db *sql.DB, fieldKey []byte) ([]MiniProgram, error) {
	programs, err := ListMiniPrograms(db, true, fieldKey)
	if err != nil {
		return nil, err
	}
	enabled := make([]MiniProgram, 0)
	for _, program := range programs {
		if program.Enabled {
			enabled = append(enabled, program)
		}
	}
	return enabled, nil
}

func CreateMiniProgram(db *sql.DB, fieldKey []byte, name, appID, appSecret string, enabled bool) (*MiniProgram, error) {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(appID) == "" || strings.TrimSpace(appSecret) == "" {
		return nil, errors.New("name, appId and appSecret are required")
	}

	encrypted, err := security.EncryptField(fieldKey, appSecret)
	if err != nil {
		return nil, err
	}
	enabledValue := 0
	if enabled {
		enabledValue = 1
	}

	result, err := db.Exec(`
INSERT INTO mini_programs (name, app_id, app_secret_encrypted, enabled)
VALUES (?, ?, ?, ?)
`, strings.TrimSpace(name), strings.TrimSpace(appID), encrypted, enabledValue)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return GetMiniProgramByID(db, id, false, fieldKey)
}

func GetMiniProgramByID(db *sql.DB, id int64, includeSecret bool, fieldKey []byte) (*MiniProgram, error) {
	var program MiniProgram
	var enabled int
	err := db.QueryRow(`
SELECT id, name, app_id, app_secret_encrypted, enabled, created_at, updated_at
FROM mini_programs
WHERE id = ?
`, id).Scan(&program.ID, &program.Name, &program.AppID, &program.encryptedSecret, &enabled, &program.CreatedAt, &program.UpdatedAt)
	if err != nil {
		return nil, err
	}
	program.Enabled = enabled == 1
	program.AppIDMasked = MaskAppID(program.AppID)
	program.AppSecretSet = program.encryptedSecret != ""
	if includeSecret && program.encryptedSecret != "" {
		secret, err := security.DecryptField(fieldKey, program.encryptedSecret)
		if err != nil {
			return nil, err
		}
		program.AppSecret = secret
	}
	return &program, nil
}

func UpdateMiniProgram(db *sql.DB, fieldKey []byte, id int64, name, appSecret string, enabled bool) (*MiniProgram, error) {
	program, err := GetMiniProgramByID(db, id, false, fieldKey)
	if err != nil {
		return nil, err
	}

	nextSecret := program.encryptedSecret
	if strings.TrimSpace(appSecret) != "" {
		nextSecret, err = security.EncryptField(fieldKey, appSecret)
		if err != nil {
			return nil, err
		}
	}
	enabledValue := 0
	if enabled {
		enabledValue = 1
	}

	_, err = db.Exec(`
UPDATE mini_programs
SET name = ?, app_secret_encrypted = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, strings.TrimSpace(name), nextSecret, enabledValue, id)
	if err != nil {
		return nil, err
	}
	return GetMiniProgramByID(db, id, false, fieldKey)
}

func DeleteMiniProgram(db *sql.DB, id int64) error {
	_, err := db.Exec("DELETE FROM mini_programs WHERE id = ?", id)
	return err
}

func SetMiniProgramEnabled(db *sql.DB, id int64, enabled bool) (*MiniProgram, error) {
	enabledValue := 0
	if enabled {
		enabledValue = 1
	}
	_, err := db.Exec(`
UPDATE mini_programs
SET enabled = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, enabledValue, id)
	if err != nil {
		return nil, err
	}
	return GetMiniProgramByID(db, id, false, nil)
}

func MaskAppID(appID string) string {
	if len(appID) <= 8 {
		return appID
	}
	return appID[:6] + "****" + appID[len(appID)-4:]
}
