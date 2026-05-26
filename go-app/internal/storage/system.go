package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type SystemUser struct {
	ID                 int64  `json:"id"`
	Username           string `json:"username"`
	PasswordHash       string `json:"-"`
	Role               string `json:"role"`
	Status             string `json:"status"`
	MustChangePassword bool   `json:"must_change_password"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
	LastLoginAt        string `json:"last_login_at,omitempty"`
}

type AuditLog struct {
	ID          int64  `json:"id"`
	UserID      *int64 `json:"user_id,omitempty"`
	Username    string `json:"username,omitempty"`
	Action      string `json:"action"`
	TargetType  string `json:"target_type,omitempty"`
	TargetID    string `json:"target_id,omitempty"`
	Description string `json:"description,omitempty"`
	Result      string `json:"result"`
	IPAddress   string `json:"ip_address,omitempty"`
	UserAgent   string `json:"user_agent,omitempty"`
	CreatedAt   string `json:"created_at"`
}

func OpenSystemDB(dataDir string) (*sql.DB, error) {
	if strings.TrimSpace(dataDir) == "" {
		dataDir = "./data"
	}
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return nil, err
	}

	dbPath := filepath.Join(dataDir, "wmam-system.db")
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", filepath.ToSlash(dbPath))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err := ApplyMigrations(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

func ApplyMigrations(db *sql.DB) error {
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`); err != nil {
		return err
	}

	for _, migration := range Migrations {
		var exists int
		if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", migration.Version).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}

		tx, err := db.Begin()
		if err != nil {
			return err
		}

		if _, err := tx.Exec(migration.SQL); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %d %s: %w", migration.Version, migration.Name, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version, name) VALUES (?, ?)", migration.Version, migration.Name); err != nil {
			_ = tx.Rollback()
			return err
		}

		if err := tx.Commit(); err != nil {
			return err
		}
	}

	return nil
}

func EnsureDefaultAdmin(db *sql.DB, passwordHash string) (bool, error) {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&count); err != nil {
		return false, err
	}
	if count > 0 {
		return false, nil
	}

	_, err := db.Exec(`
INSERT INTO users (username, password_hash, role, status, must_change_password, password_changed_at)
VALUES ('admin', ?, 'admin', 'active', 1, CURRENT_TIMESTAMP)
`, passwordHash)
	return err == nil, err
}

func GetAdminUser(db *sql.DB) (*SystemUser, error) {
	return scanSystemUser(db.QueryRow(`
SELECT id, username, password_hash, role, status, must_change_password,
       created_at, updated_at, COALESCE(last_login_at, '')
FROM users
WHERE role = 'admin'
LIMIT 1
`))
}

func EnsureAdminRecoveryHash(db *sql.DB, recoveryHash string) (bool, error) {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM admin_recovery WHERE id = 1").Scan(&count); err != nil {
		return false, err
	}
	if count > 0 {
		return false, nil
	}
	_, err := db.Exec(`
INSERT INTO admin_recovery (id, recovery_hash)
VALUES (1, ?)
`, recoveryHash)
	return err == nil, err
}

func GetAdminRecoveryHash(db *sql.DB) (string, error) {
	var hash string
	err := db.QueryRow("SELECT recovery_hash FROM admin_recovery WHERE id = 1").Scan(&hash)
	return hash, err
}

func ReplaceAdminRecoveryHash(db *sql.DB, recoveryHash string) error {
	_, err := db.Exec(`
INSERT INTO admin_recovery (id, recovery_hash, created_at, used_at)
VALUES (1, ?, CURRENT_TIMESTAMP, NULL)
ON CONFLICT(id) DO UPDATE SET
    recovery_hash = excluded.recovery_hash,
    created_at = CURRENT_TIMESTAMP,
    used_at = NULL
`, recoveryHash)
	return err
}

func GetUserByID(db *sql.DB, id int64) (*SystemUser, error) {
	return scanSystemUser(db.QueryRow(`
SELECT id, username, password_hash, role, status, must_change_password,
       created_at, updated_at, COALESCE(last_login_at, '')
FROM users
WHERE id = ?
`, id))
}

func GetUserByUsername(db *sql.DB, username string) (*SystemUser, error) {
	return scanSystemUser(db.QueryRow(`
SELECT id, username, password_hash, role, status, must_change_password,
       created_at, updated_at, COALESCE(last_login_at, '')
FROM users
WHERE username = ?
`, strings.TrimSpace(username)))
}

func ListUsers(db *sql.DB) ([]SystemUser, error) {
	rows, err := db.Query(`
SELECT id, username, password_hash, role, status, must_change_password,
       created_at, updated_at, COALESCE(last_login_at, '')
FROM users
ORDER BY created_at DESC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]SystemUser, 0)
	for rows.Next() {
		user, err := scanUserRows(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, *user)
	}
	return users, rows.Err()
}

func CreateUser(db *sql.DB, username, passwordHash, role string) (*SystemUser, error) {
	role = strings.TrimSpace(role)
	if role != "admin" && role != "user" {
		return nil, errors.New("invalid role")
	}
	if role == "admin" {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&count); err != nil {
			return nil, err
		}
		if count > 0 {
			return nil, errors.New("admin already exists")
		}
	}

	result, err := db.Exec(`
INSERT INTO users (username, password_hash, role, status, must_change_password, password_changed_at)
VALUES (?, ?, ?, 'active', 1, CURRENT_TIMESTAMP)
`, strings.TrimSpace(username), passwordHash, role)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return GetUserByID(db, id)
}

func UpdateUser(db *sql.DB, id int64, username, status string) (*SystemUser, error) {
	status = strings.TrimSpace(status)
	if status != "active" && status != "disabled" {
		return nil, errors.New("invalid status")
	}

	user, err := GetUserByID(db, id)
	if err != nil {
		return nil, err
	}
	if user.Role == "admin" && status != "active" {
		return nil, errors.New("admin cannot be disabled")
	}

	_, err = db.Exec(`
UPDATE users
SET username = ?, status = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, strings.TrimSpace(username), status, id)
	if err != nil {
		return nil, err
	}
	return GetUserByID(db, id)
}

func UpdateUserPassword(db *sql.DB, id int64, passwordHash string, mustChangePassword bool) error {
	mustChange := 0
	if mustChangePassword {
		mustChange = 1
	}
	_, err := db.Exec(`
UPDATE users
SET password_hash = ?,
    must_change_password = ?,
    password_changed_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, passwordHash, mustChange, id)
	return err
}

func DeleteUser(db *sql.DB, id int64) error {
	user, err := GetUserByID(db, id)
	if err != nil {
		return err
	}
	if user.Role == "admin" {
		return errors.New("admin cannot be deleted")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM sessions WHERE user_id = ?", id); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec("DELETE FROM users WHERE id = ?", id); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func UpdateLastLogin(db *sql.DB, id int64) error {
	_, err := db.Exec(`
UPDATE users
SET last_login_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, id)
	return err
}

func ValidateSession(db *sql.DB, tokenHash string) (bool, error) {
	var expiresAtRaw string
	err := db.QueryRow(`
SELECT expires_at
FROM sessions
WHERE token_hash = ? AND revoked_at IS NULL
`, tokenHash).Scan(&expiresAtRaw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	expiresAt, err := time.Parse(time.RFC3339, expiresAtRaw)
	if err != nil {
		return false, err
	}
	if time.Now().After(expiresAt) {
		return false, nil
	}

	_, _ = db.Exec("UPDATE sessions SET last_used_at = CURRENT_TIMESTAMP WHERE token_hash = ?", tokenHash)
	return true, nil
}

func CreateSession(db *sql.DB, userID int64, tokenHash string, remember bool, expiresAt time.Time, ipAddress, userAgent string) error {
	rememberValue := 0
	if remember {
		rememberValue = 1
	}
	_, err := db.Exec(`
INSERT INTO sessions (user_id, token_hash, remember, expires_at, ip_address, user_agent)
VALUES (?, ?, ?, ?, ?, ?)
`, userID, tokenHash, rememberValue, expiresAt.Format(time.RFC3339), ipAddress, userAgent)
	return err
}

func RevokeUserSessions(db *sql.DB, userID int64) error {
	_, err := db.Exec(`
UPDATE sessions
SET revoked_at = CURRENT_TIMESTAMP
WHERE user_id = ? AND revoked_at IS NULL
`, userID)
	return err
}

func RevokeAllSessions(db *sql.DB) error {
	_, err := db.Exec(`
UPDATE sessions
SET revoked_at = CURRENT_TIMESTAMP
WHERE revoked_at IS NULL
`)
	return err
}

func RevokeSession(db *sql.DB, tokenHash string) error {
	_, err := db.Exec(`
UPDATE sessions
SET revoked_at = CURRENT_TIMESTAMP
WHERE token_hash = ? AND revoked_at IS NULL
`, tokenHash)
	return err
}

func CreateAuditLog(db *sql.DB, userID *int64, username, action, targetType, targetID, description, result, ipAddress, userAgent string) error {
	if result == "" {
		result = "success"
	}
	_, err := db.Exec(`
INSERT INTO audit_logs (user_id, username, action, target_type, target_id, description, result, ip_address, user_agent)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
`, userID, username, action, targetType, targetID, description, result, ipAddress, userAgent)
	return err
}

func ListAuditLogs(db *sql.DB, userID int64, isAdmin bool, offset, limit int) ([]AuditLog, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var count int64
	var rows *sql.Rows
	var err error
	if isAdmin {
		err = db.QueryRow("SELECT COUNT(*) FROM audit_logs").Scan(&count)
		if err == nil {
			rows, err = db.Query(`
SELECT id, user_id, COALESCE(username, ''), action, COALESCE(target_type, ''),
       COALESCE(target_id, ''), COALESCE(description, ''), result,
       COALESCE(ip_address, ''), COALESCE(user_agent, ''), created_at
FROM audit_logs
ORDER BY created_at DESC
LIMIT ? OFFSET ?
`, limit, offset)
		}
	} else {
		err = db.QueryRow("SELECT COUNT(*) FROM audit_logs WHERE user_id = ?", userID).Scan(&count)
		if err == nil {
			rows, err = db.Query(`
SELECT id, user_id, COALESCE(username, ''), action, COALESCE(target_type, ''),
       COALESCE(target_id, ''), COALESCE(description, ''), result,
       COALESCE(ip_address, ''), COALESCE(user_agent, ''), created_at
FROM audit_logs
WHERE user_id = ?
ORDER BY created_at DESC
LIMIT ? OFFSET ?
`, userID, limit, offset)
		}
	}
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	logs := make([]AuditLog, 0)
	for rows.Next() {
		var item AuditLog
		var nullableUserID sql.NullInt64
		if err := rows.Scan(
			&item.ID,
			&nullableUserID,
			&item.Username,
			&item.Action,
			&item.TargetType,
			&item.TargetID,
			&item.Description,
			&item.Result,
			&item.IPAddress,
			&item.UserAgent,
			&item.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		if nullableUserID.Valid {
			item.UserID = &nullableUserID.Int64
		}
		item.CreatedAt = formatDisplayTime(item.CreatedAt)
		logs = append(logs, item)
	}

	return logs, count, rows.Err()
}

func CountUsers(db *sql.DB) (int64, error) {
	var count int64
	err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}

func CountMiniPrograms(db *sql.DB) (int64, error) {
	var count int64
	err := db.QueryRow("SELECT COUNT(*) FROM mini_programs").Scan(&count)
	return count, err
}

func CountTodayFetchJobs(db *sql.DB) (int64, error) {
	var count int64
	today := time.Now().Format("2006-01-02")
	err := db.QueryRow("SELECT COUNT(*) FROM fetch_jobs WHERE date(started_at) = ?", today).Scan(&count)
	return count, err
}

func LastFetchTime(db *sql.DB) (string, error) {
	var value string
	err := db.QueryRow("SELECT COALESCE(MAX(started_at), '') FROM fetch_jobs").Scan(&value)
	return formatDisplayTime(value), err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSystemUser(row rowScanner) (*SystemUser, error) {
	user := &SystemUser{}
	var mustChange int
	err := row.Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
		&user.Role,
		&user.Status,
		&mustChange,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.LastLoginAt,
	)
	if err != nil {
		return nil, err
	}
	user.MustChangePassword = mustChange == 1
	user.CreatedAt = formatDisplayTime(user.CreatedAt)
	user.UpdatedAt = formatDisplayTime(user.UpdatedAt)
	user.LastLoginAt = formatDisplayTime(user.LastLoginAt)
	return user, nil
}

func scanUserRows(rows *sql.Rows) (*SystemUser, error) {
	return scanSystemUser(rows)
}
