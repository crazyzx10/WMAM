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
	if _, err := CreateUser(db, "second-admin", "hashed-password", "admin"); err == nil {
		t.Fatal("expected creating a second admin to fail")
	}

	if err := CreateSession(db, admin.ID, "token-hash", true, time.Now().Add(30*24*time.Hour), "127.0.0.1", "test"); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if err := RevokeSession(db, "token-hash"); err != nil {
		t.Fatalf("RevokeSession() error = %v", err)
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
