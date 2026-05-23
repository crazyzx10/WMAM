package models

import (
	"database/sql"
	"time"
)

// FetchProgress 拉取进度模型
type FetchProgress struct {
	ID                 int       `json:"id" db:"id"`
	CurrentProgramIndex int      `json:"current_program_index" db:"current_program_index"`
	ProgramNames       string    `json:"program_names" db:"program_names"`
	ProgramIDs         string    `json:"program_ids" db:"program_ids"`
	AdunitListStatus   string    `json:"adunit_list_status" db:"adunit_list_status"`
	SummaryStatus      string    `json:"summary_status" db:"summary_status"`
	DetailStatus       string    `json:"detail_status" db:"detail_status"`
	SettlementStatus   string    `json:"settlement_status" db:"settlement_status"`
	CurrentDataType    string    `json:"current_data_type" db:"current_data_type"`
	LockedBy           string    `json:"locked_by" db:"locked_by"`
	LockedAt           time.Time `json:"locked_at" db:"locked_at"`
	UpdatedAt          time.Time `json:"updated_at" db:"updated_at"`
}

// GetFetchProgress 获取拉取进度
func GetFetchProgress(db *sql.DB) (*FetchProgress, error) {
	var progress FetchProgress
	err := db.QueryRow(`
		SELECT id, current_program_index, program_names, program_ids, 
		       adunit_list_status, summary_status, detail_status, settlement_status,
		       current_data_type, locked_by, locked_at, updated_at
		FROM fetch_progress WHERE id = 1
	`).Scan(
		&progress.ID, &progress.CurrentProgramIndex, &progress.ProgramNames, &progress.ProgramIDs,
		&progress.AdunitListStatus, &progress.SummaryStatus, &progress.DetailStatus, &progress.SettlementStatus,
		&progress.CurrentDataType, &progress.LockedBy, &progress.LockedAt, &progress.UpdatedAt,
	)
	if err != nil {
		// 如果不存在，初始化一个
		if err == sql.ErrNoRows {
			_, err := db.Exec(`
				INSERT INTO fetch_progress (id, current_program_index, 
					adunit_list_status, summary_status, detail_status, settlement_status)
				VALUES (1, 0, 'pending', 'pending', 'pending', 'pending')
			`)
			if err != nil {
				return nil, err
			}
			return GetFetchProgress(db)
		}
		return nil, err
	}
	return &progress, nil
}

// ResetFetchProgress 重置拉取进度
func ResetFetchProgress(db *sql.DB, programNames, programIDs string) error {
	_, err := db.Exec(`
		UPDATE fetch_progress 
		SET current_program_index = 0, program_names = ?, program_ids = ?,
		    adunit_list_status = 'pending', summary_status = 'pending',
		    detail_status = 'pending', settlement_status = 'pending',
		    current_data_type = '', locked_by = '', updated_at = CURRENT_TIMESTAMP
		WHERE id = 1
	`, programNames, programIDs)
	return err
}

// UpdateFetchProgress 更新拉取进度
func UpdateFetchProgress(db *sql.DB, currentProgramIndex int, dataType, status string) error {
	var err error
	switch dataType {
	case "adunit_list":
		_, err = db.Exec(`
			UPDATE fetch_progress 
			SET current_program_index = ?, current_data_type = ?, adunit_list_status = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = 1
		`, currentProgramIndex, dataType, status)
	case "summary":
		_, err = db.Exec(`
			UPDATE fetch_progress 
			SET current_program_index = ?, current_data_type = ?, summary_status = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = 1
		`, currentProgramIndex, dataType, status)
	case "detail":
		_, err = db.Exec(`
			UPDATE fetch_progress 
			SET current_program_index = ?, current_data_type = ?, detail_status = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = 1
		`, currentProgramIndex, dataType, status)
	case "settlement":
		_, err = db.Exec(`
			UPDATE fetch_progress 
			SET current_program_index = ?, current_data_type = ?, settlement_status = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = 1
		`, currentProgramIndex, dataType, status)
	}
	return err
}

// LockFetchProgress 锁定拉取进度
func LockFetchProgress(db *sql.DB, username string) error {
	_, err := db.Exec(`
		UPDATE fetch_progress 
		SET locked_by = ?, locked_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = 1
	`, username)
	return err
}

// UnlockFetchProgress 解锁拉取进度
func UnlockFetchProgress(db *sql.DB) error {
	_, err := db.Exec(`
		UPDATE fetch_progress 
		SET locked_by = '', updated_at = CURRENT_TIMESTAMP
		WHERE id = 1
	`)
	return err
}
