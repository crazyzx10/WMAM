package storage

import (
	"testing"
	"time"
)

func TestSystemStorageFoundation(t *testing.T) {
	db, err := OpenSystemDB(t.TempDir())
	if err != nil {
		t.Fatalf("OpenSystemDB() error = %v", err)
	}
	defer db.Close()

	created, err := EnsureDefaultAdmin(db, "hashed-password")
	if err != nil {
		t.Fatalf("EnsureDefaultAdmin() error = %v", err)
	}
	if !created {
		t.Fatal("expected default admin to be created")
	}

	created, err = EnsureDefaultAdmin(db, "another-password")
	if err != nil {
		t.Fatalf("EnsureDefaultAdmin() second call error = %v", err)
	}
	if created {
		t.Fatal("expected second default admin creation to be skipped")
	}

	admin, err := GetUserByUsername(db, "admin")
	if err != nil {
		t.Fatalf("GetUserByUsername() error = %v", err)
	}
	if admin.Role != "admin" || admin.Status != "active" {
		t.Fatalf("unexpected admin state: role=%s status=%s", admin.Role, admin.Status)
	}

	if _, err := CreateUser(db, "alice", "hashed-password", "user"); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	alice, err := GetUserByUsername(db, "alice")
	if err != nil {
		t.Fatalf("GetUserByUsername(alice) error = %v", err)
	}
	if !alice.MustChangePassword {
		t.Fatal("expected new ordinary user to require password change")
	}
	if _, err := CreateUser(db, "second-admin", "hashed-password", "admin"); err == nil {
		t.Fatal("expected creating a second admin to fail")
	}

	if err := CreateSession(db, admin.ID, "token-hash", true, time.Now().Add(30*24*time.Hour), "127.0.0.1", "test"); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if err := RevokeSession(db, "token-hash"); err != nil {
		t.Fatalf("RevokeSession() error = %v", err)
	}
	active, err := ValidateSession(db, "token-hash")
	if err != nil {
		t.Fatalf("ValidateSession() error = %v", err)
	}
	if active {
		t.Fatal("expected revoked session to be inactive")
	}

	if err := UpdateUserPassword(db, alice.ID, "new-hash", false); err != nil {
		t.Fatalf("UpdateUserPassword() error = %v", err)
	}
	alice, err = GetUserByID(db, alice.ID)
	if err != nil {
		t.Fatalf("GetUserByID(alice) error = %v", err)
	}
	if alice.MustChangePassword {
		t.Fatal("expected password change requirement to be cleared")
	}

	if err := CreateAuditLog(db, &admin.ID, admin.Username, "LOGIN", "user", "1", "登录成功", "success", "127.0.0.1", "test"); err != nil {
		t.Fatalf("CreateAuditLog() error = %v", err)
	}

	logs, total, err := ListAuditLogs(db, admin.ID, true, 0, 20)
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}
	if total != 1 || len(logs) != 1 || logs[0].Action != "LOGIN" {
		t.Fatalf("unexpected audit logs: total=%d logs=%v", total, logs)
	}
}

func TestConfigAndMiniPrograms(t *testing.T) {
	db, err := OpenSystemDB(t.TempDir())
	if err != nil {
		t.Fatalf("OpenSystemDB() error = %v", err)
	}
	defer db.Close()

	fieldKey := []byte("12345678901234567890123456789012")
	if err := SaveMySQLConfig(db, fieldKey, MySQLConfig{
		Host:     "127.0.0.1",
		Port:     3306,
		Database: "wmam",
		Username: "root",
		Password: "secret",
	}); err != nil {
		t.Fatalf("SaveMySQLConfig() error = %v", err)
	}

	displayConfig, err := GetMySQLConfig(db, fieldKey, false)
	if err != nil {
		t.Fatalf("GetMySQLConfig(display) error = %v", err)
	}
	if displayConfig.Password != "" || !displayConfig.PasswordSet {
		t.Fatalf("expected password to be hidden and marked as set: %+v", displayConfig)
	}

	fullConfig, err := GetMySQLConfig(db, fieldKey, true)
	if err != nil {
		t.Fatalf("GetMySQLConfig(full) error = %v", err)
	}
	if fullConfig.Password != "secret" {
		t.Fatalf("expected decrypted mysql password, got %q", fullConfig.Password)
	}

	program, err := CreateMiniProgram(db, fieldKey, "Demo", "wx1234567890abcd", "app-secret", true)
	if err != nil {
		t.Fatalf("CreateMiniProgram() error = %v", err)
	}
	if !program.AppSecretSet || program.AppSecret != "" {
		t.Fatalf("expected secret to be hidden in create response: %+v", program)
	}

	programs, err := ListEnabledMiniProgramsWithSecret(db, fieldKey)
	if err != nil {
		t.Fatalf("ListEnabledMiniProgramsWithSecret() error = %v", err)
	}
	if len(programs) != 1 || programs[0].AppSecret != "app-secret" {
		t.Fatalf("expected decrypted enabled program secret, got %+v", programs)
	}

	if _, err := UpdateMiniProgram(db, fieldKey, program.ID, "Demo Updated", "", false); err != nil {
		t.Fatalf("UpdateMiniProgram() error = %v", err)
	}
	programs, err = ListEnabledMiniProgramsWithSecret(db, fieldKey)
	if err != nil {
		t.Fatalf("ListEnabledMiniProgramsWithSecret(disabled) error = %v", err)
	}
	if len(programs) != 0 {
		t.Fatalf("expected disabled program to be excluded, got %+v", programs)
	}
}
