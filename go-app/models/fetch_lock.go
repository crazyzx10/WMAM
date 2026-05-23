package models

import (
	"database/sql"
	"time"
)

// FetchLock 拉取锁模型
type FetchLock struct {
	ID         int       `json:"id" db:"id"`
	LockedBy   string    `json:"locked_by" db:"locked_by"`
	LockedAt   time.Time `json:"locked_at" db:"locked_at"`
	ExpiresAt  time.Time `json:"expires_at" db:"expires_at"`
}

// GetFetchLock 获取拉取锁
func GetFetchLock(db *sql.DB) (*FetchLock, error) {
	var lock FetchLock
	err := db.QueryRow(`
		SELECT id, locked_by, locked_at, expires_at FROM fetch_lock WHERE id = 1
	`).Scan(&lock.ID, &lock.LockedBy, &lock.LockedAt, &lock.ExpiresAt)
	if err != nil {
		// 如果不存在，初始化一个
		if err == sql.ErrNoRows {
			_, err := db.Exec("INSERT INTO fetch_lock (id) VALUES (1)")
			if err != nil {
				return nil, err
			}
			return GetFetchLock(db)
		}
		return nil, err
	}
	return &lock, nil
}

// AcquireFetchLock 获取拉取锁
func AcquireFetchLock(db *sql.DB, username string) (bool, error) {
	lock, err := GetFetchLock(db)
	if err != nil {
		return false, err
	}

	now := time.Now()
	// 检查是否已过期
	if lock.LockedBy != "" && lock.ExpiresAt.After(now) {
		return false, nil
	}

	expiresAt := now.Add(30 * time.Minute)
	result, err := db.Exec(`
		UPDATE fetch_lock 
		SET locked_by = ?, locked_at = ?, expires_at = ? 
		WHERE id = 1
	`, username, now, expiresAt)
	if err != nil {
		return false, err
	}

	rowsAffected, _ := result.RowsAffected()
	return rowsAffected > 0, nil
}

// ReleaseFetchLock 释放拉取锁
func ReleaseFetchLock(db *sql.DB, username string) (bool, error) {
	result, err := db.Exec(`
		UPDATE fetch_lock 
		SET locked_by = '', locked_at = NULL, expires_at = NULL 
		WHERE id = 1 AND locked_by = ?
	`, username)
	if err != nil {
		return false, err
	}
	rowsAffected, _ := result.RowsAffected()
	return rowsAffected > 0, nil
}
