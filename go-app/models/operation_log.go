package models

import (
	"database/sql"
	"time"
)

// OperationLog 操作日志模型
type OperationLog struct {
	ID            int64     `json:"id" db:"id"`
	UserID        int64     `json:"user_id" db:"user_id"`
	Username      string    `json:"username" db:"username"`
	OperationType string    `json:"operation_type" db:"operation_type"`
	OperationDesc string    `json:"operation_desc" db:"operation_desc"`
	IPAddress     string    `json:"ip_address" db:"ip_address"`
	UserAgent     string    `json:"user_agent" db:"user_agent"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

// CreateOperationLog 创建操作日志
func CreateOperationLog(db *sql.DB, userID int64, username, operationType, operationDesc, ipAddress, userAgent string) error {
	_, err := db.Exec(`
		INSERT INTO operation_log (user_id, username, operation_type, operation_desc, ip_address, user_agent) 
		VALUES (?, ?, ?, ?, ?, ?)
	`, userID, username, operationType, operationDesc, ipAddress, userAgent)
	return err
}

// GetOperationLogs 获取操作日志列表（管理员获取所有，普通用户只获取自己的）
func GetOperationLogs(db *sql.DB, userID int64, isAdmin bool, offset, limit int) ([]*OperationLog, int64, error) {
	var rows *sql.Rows
	var err error
	var count int64

	if isAdmin {
		err = db.QueryRow("SELECT COUNT(*) FROM operation_log").Scan(&count)
		rows, err = db.Query(`
			SELECT id, user_id, username, operation_type, operation_desc, ip_address, user_agent, created_at 
			FROM operation_log ORDER BY created_at DESC LIMIT ? OFFSET ?
		`, limit, offset)
	} else {
		err = db.QueryRow("SELECT COUNT(*) FROM operation_log WHERE user_id = ?", userID).Scan(&count)
		rows, err = db.Query(`
			SELECT id, user_id, username, operation_type, operation_desc, ip_address, user_agent, created_at 
			FROM operation_log WHERE user_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?
		`, userID, limit, offset)
	}

	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []*OperationLog
	for rows.Next() {
		var log OperationLog
		err := rows.Scan(
			&log.ID, &log.UserID, &log.Username, &log.OperationType,
			&log.OperationDesc, &log.IPAddress, &log.UserAgent, &log.CreatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		logs = append(logs, &log)
	}
	return logs, count, nil
}

// GetTodayFetchCount 获取今日拉取次数
func GetTodayFetchCount(db *sql.DB) (int64, error) {
	var count int64
	today := time.Now().Format("2006-01-02")
	err := db.QueryRow(`
		SELECT COUNT(*) FROM operation_log 
		WHERE operation_type = 'FETCH_START' AND DATE(created_at) = ?
	`, today).Scan(&count)
	return count, err
}

// GetLastFetchTime 获取最后拉取时间
func GetLastFetchTime(db *sql.DB) (*time.Time, error) {
	var lastTime sql.NullTime
	err := db.QueryRow(`
		SELECT created_at FROM operation_log 
		WHERE operation_type IN ('FETCH_SUCCESS', 'FETCH_START') 
		ORDER BY created_at DESC LIMIT 1
	`).Scan(&lastTime)
	if err != nil {
		return nil, err
	}
	if !lastTime.Valid {
		return nil, nil
	}
	return &lastTime.Time, nil
}
