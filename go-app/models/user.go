package models

import (
	"database/sql"
	"time"
)

// User 用户模型
type User struct {
	ID          int64     `json:"id" db:"id"`
	Username    string    `json:"username" db:"username"`
	PasswordHash string   `json:"-" db:"password_hash"`
	Role        string    `json:"role" db:"role"`
	Status      string    `json:"status" db:"status"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty" db:"last_login_at"`
}

// CreateUser 创建用户
func CreateUser(db *sql.DB, username, passwordHash, role string) (*User, error) {
	result, err := db.Exec(`
		INSERT INTO users (username, password_hash, role, status) 
		VALUES (?, ?, ?, 'active')
	`, username, passwordHash, role)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return GetUserByID(db, id)
}

// GetUserByID 根据ID获取用户
func GetUserByID(db *sql.DB, id int64) (*User, error) {
	var user User
	err := db.QueryRow(`
		SELECT id, username, password_hash, role, status, created_at, updated_at, last_login_at 
		FROM users WHERE id = ?
	`, id).Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.Status,
		&user.CreatedAt, &user.UpdatedAt, &user.LastLoginAt,
	)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetUserByUsername 根据用户名获取用户
func GetUserByUsername(db *sql.DB, username string) (*User, error) {
	var user User
	err := db.QueryRow(`
		SELECT id, username, password_hash, role, status, created_at, updated_at, last_login_at 
		FROM users WHERE username = ?
	`, username).Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.Status,
		&user.CreatedAt, &user.UpdatedAt, &user.LastLoginAt,
	)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetAllUsers 获取所有用户
func GetAllUsers(db *sql.DB) ([]*User, error) {
	rows, err := db.Query(`
		SELECT id, username, password_hash, role, status, created_at, updated_at, last_login_at 
		FROM users ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		var user User
		err := rows.Scan(
			&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.Status,
			&user.CreatedAt, &user.UpdatedAt, &user.LastLoginAt,
		)
		if err != nil {
			return nil, err
		}
		users = append(users, &user)
	}
	return users, nil
}

// UpdateUser 更新用户
func UpdateUser(db *sql.DB, id int64, username, status string) (*User, error) {
	_, err := db.Exec(`
		UPDATE users SET username = ?, status = ?, updated_at = CURRENT_TIMESTAMP 
		WHERE id = ?
	`, username, status, id)
	if err != nil {
		return nil, err
	}
	return GetUserByID(db, id)
}

// UpdateUserPassword 更新用户密码
func UpdateUserPassword(db *sql.DB, id int64, passwordHash string) error {
	_, err := db.Exec(`
		UPDATE users SET password_hash = ?, updated_at = CURRENT_TIMESTAMP 
		WHERE id = ?
	`, passwordHash, id)
	return err
}

// DeleteUser 删除用户
func DeleteUser(db *sql.DB, id int64) error {
	_, err := db.Exec("DELETE FROM users WHERE id = ?", id)
	return err
}

// UpdateLastLogin 更新最后登录时间
func UpdateLastLogin(db *sql.DB, id int64) error {
	_, err := db.Exec(`
		UPDATE users SET last_login_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP 
		WHERE id = ?
	`, id)
	return err
}

// GetUserCount 获取用户总数
func GetUserCount(db *sql.DB) (int64, error) {
	var count int64
	err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}
