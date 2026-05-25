package storage

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"golang.org/x/crypto/scrypt"
)

const backupVersion = 1

var backupTables = []string{
	"system_meta",
	"users",
	"admin_recovery",
	"system_config",
	"mini_programs",
	"fetch_jobs",
	"fetch_job_steps",
	"fetch_lock",
	"audit_logs",
}

type backupEnvelope struct {
	Version    int    `json:"version"`
	KDF        string `json:"kdf"`
	Salt       string `json:"salt"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
	CreatedAt  string `json:"createdAt"`
}

type backupPayload struct {
	Version  int                         `json:"version"`
	FieldKey string                      `json:"fieldKey"`
	Tables   map[string][]map[string]any `json:"tables"`
}

func ExportEncryptedBackup(db *sql.DB, fieldKey []byte, password string) ([]byte, error) {
	if strings.TrimSpace(password) == "" {
		return nil, errors.New("backup password is required")
	}

	tables := make(map[string][]map[string]any, len(backupTables))
	for _, table := range backupTables {
		rows, err := readTableRows(db, table)
		if err != nil {
			return nil, err
		}
		tables[table] = rows
	}

	payload, err := json.Marshal(backupPayload{
		Version:  backupVersion,
		FieldKey: base64.RawStdEncoding.EncodeToString(fieldKey),
		Tables:   tables,
	})
	if err != nil {
		return nil, err
	}

	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	key, err := scrypt.Key([]byte(password), salt, 1<<15, 8, 1, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	envelope := backupEnvelope{
		Version:    backupVersion,
		KDF:        "scrypt-N32768-r8-p1",
		Salt:       base64.RawStdEncoding.EncodeToString(salt),
		Nonce:      base64.RawStdEncoding.EncodeToString(nonce),
		Ciphertext: base64.RawStdEncoding.EncodeToString(gcm.Seal(nil, nonce, payload, nil)),
		CreatedAt:  time.Now().Format(time.RFC3339),
	}
	return json.MarshalIndent(envelope, "", "  ")
}

func ImportEncryptedBackup(db *sql.DB, data []byte, password string) ([]byte, error) {
	if strings.TrimSpace(password) == "" {
		return nil, errors.New("backup password is required")
	}

	var envelope backupEnvelope
	if err := json.Unmarshal(bytes.TrimSpace(data), &envelope); err != nil {
		return nil, err
	}
	if envelope.Version != backupVersion {
		return nil, errors.New("unsupported backup version")
	}

	salt, err := base64.RawStdEncoding.DecodeString(envelope.Salt)
	if err != nil {
		return nil, err
	}
	nonce, err := base64.RawStdEncoding.DecodeString(envelope.Nonce)
	if err != nil {
		return nil, err
	}
	ciphertext, err := base64.RawStdEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		return nil, err
	}
	key, err := scrypt.Key([]byte(password), salt, 1<<15, 8, 1, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	rawPayload, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, errors.New("invalid backup password or backup file")
	}

	var payload backupPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, err
	}
	if payload.Version != backupVersion {
		return nil, errors.New("unsupported backup payload version")
	}
	fieldKey, err := base64.RawStdEncoding.DecodeString(payload.FieldKey)
	if err != nil {
		return nil, err
	}
	if len(fieldKey) != 32 {
		return nil, errors.New("invalid field key in backup")
	}

	if err := replaceTables(db, payload.Tables); err != nil {
		return nil, err
	}
	if err := normalizeImportedRuntimeState(db); err != nil {
		return nil, err
	}
	return fieldKey, nil
}

func readTableRows(db *sql.DB, table string) ([]map[string]any, error) {
	rows, err := db.Query("SELECT * FROM " + quoteIdent(table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	items := make([]map[string]any, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		dest := make([]any, len(columns))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}

		item := make(map[string]any, len(columns))
		for i, column := range columns {
			switch value := values[i].(type) {
			case []byte:
				item[column] = string(value)
			default:
				item[column] = value
			}
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func replaceTables(db *sql.DB, tables map[string][]map[string]any) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	deleteOrder := []string{"sessions", "fetch_lock", "fetch_job_steps", "fetch_jobs", "audit_logs", "mini_programs", "system_config", "admin_recovery", "users", "system_meta"}
	for _, table := range deleteOrder {
		if _, err := tx.Exec("DELETE FROM " + quoteIdent(table)); err != nil {
			_ = tx.Rollback()
			return err
		}
	}

	for _, table := range backupTables {
		for _, row := range tables[table] {
			if err := insertBackupRow(tx, table, row); err != nil {
				_ = tx.Rollback()
				return err
			}
		}
	}

	return tx.Commit()
}

func normalizeImportedRuntimeState(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec("DELETE FROM sessions"); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`
UPDATE fetch_job_steps
SET status = 'failed',
    finished_at = COALESCE(finished_at, CURRENT_TIMESTAMP),
    error_message = CASE WHEN COALESCE(error_message, '') = '' THEN '备份恢复时任务仍在运行，已标记为失败' ELSE error_message END,
    updated_at = CURRENT_TIMESTAMP
WHERE status = 'running'
`); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`
UPDATE fetch_jobs
SET status = 'failed',
    error_summary = CASE WHEN COALESCE(error_summary, '') = '' THEN '备份恢复时任务仍在运行，已标记为失败' ELSE error_summary END,
    finished_at = COALESCE(finished_at, CURRENT_TIMESTAMP),
    updated_at = CURRENT_TIMESTAMP
WHERE status = 'running'
`); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec("INSERT INTO fetch_lock (id) VALUES (1) ON CONFLICT(id) DO NOTHING"); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`
UPDATE fetch_lock
SET job_id = NULL,
    locked_by_user_id = NULL,
    locked_by_username = NULL,
    locked_at = NULL,
    heartbeat_at = NULL,
    expires_at = NULL
WHERE id = 1
`); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

func insertBackupRow(tx *sql.Tx, table string, row map[string]any) error {
	if len(row) == 0 {
		return nil
	}

	columns := make([]string, 0, len(row))
	placeholders := make([]string, 0, len(row))
	values := make([]any, 0, len(row))
	for column, value := range row {
		columns = append(columns, quoteIdent(column))
		placeholders = append(placeholders, "?")
		values = append(values, value)
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", quoteIdent(table), strings.Join(columns, ","), strings.Join(placeholders, ","))
	_, err := tx.Exec(query, values...)
	return err
}

func quoteIdent(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
