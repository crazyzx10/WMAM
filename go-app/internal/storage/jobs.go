package storage

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	jobstate "go-app/internal/jobs"
)

type FetchJob struct {
	ID                 int64  `json:"id"`
	Status             string `json:"status"`
	StartedByUserID    int64  `json:"startedByUserId"`
	StartedByUsername  string `json:"startedByUsername"`
	StartedAt          string `json:"startedAt"`
	FinishedAt         string `json:"finishedAt,omitempty"`
	CurrentProgramID   *int64 `json:"currentProgramId,omitempty"`
	CurrentProgramName string `json:"currentProgramName,omitempty"`
	CurrentStep        string `json:"currentStep,omitempty"`
	TotalPrograms      int    `json:"totalPrograms"`
	TotalSteps         int    `json:"totalSteps"`
	CompletedSteps     int    `json:"completedSteps"`
	FailedSteps        int    `json:"failedSteps"`
	ProgressPercent    int    `json:"progressPercent"`
	ErrorSummary       string `json:"errorSummary,omitempty"`
	InterruptRequested bool   `json:"interruptRequested"`
	EndRequested       bool   `json:"endRequested"`
	CreatedAt          string `json:"createdAt"`
	UpdatedAt          string `json:"updatedAt"`
}

type FetchJobStep struct {
	ID           int64  `json:"id"`
	JobID        int64  `json:"jobId"`
	ProgramID    int64  `json:"programId"`
	ProgramName  string `json:"programName"`
	AppIDMasked  string `json:"appIdMasked"`
	StepType     string `json:"stepType"`
	Status       string `json:"status"`
	StartedAt    string `json:"startedAt,omitempty"`
	FinishedAt   string `json:"finishedAt,omitempty"`
	RecordCount  int    `json:"recordCount"`
	ErrorMessage string `json:"errorMessage,omitempty"`
	RetryCount   int    `json:"retryCount"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

type JobPermissions struct {
	CanStart     bool `json:"canStart"`
	CanInterrupt bool `json:"canInterrupt"`
	CanResume    bool `json:"canResume"`
	CanEnd       bool `json:"canEnd"`
}

const FetchJobLockTTL = 30 * time.Minute

type fetchJobLockExecutor interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
}

func fetchJobLockExpiresAt(ttl time.Duration) string {
	return time.Now().Add(ttl).UTC().Format(time.RFC3339)
}

func expireStaleFetchLocks(exec fetchJobLockExecutor) error {
	now := time.Now().UTC().Format(time.RFC3339)

	var jobID sql.NullInt64
	err := exec.QueryRow(`
SELECT job_id
FROM fetch_lock
WHERE id = 1
  AND job_id IS NOT NULL
  AND expires_at IS NOT NULL
  AND expires_at < ?
`, now).Scan(&jobID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if !jobID.Valid {
		return nil
	}

	if _, err := exec.Exec(`
UPDATE fetch_job_steps
SET status = 'failed',
    finished_at = COALESCE(finished_at, CURRENT_TIMESTAMP),
    error_message = CASE WHEN COALESCE(error_message, '') = '' THEN '任务锁已过期' ELSE error_message END,
    updated_at = CURRENT_TIMESTAMP
WHERE job_id = ? AND status = 'running'
`, jobID.Int64); err != nil {
		return err
	}

	var total, completed, failed int
	if err := exec.QueryRow(`
SELECT COUNT(*),
       COALESCE(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0)
FROM fetch_job_steps
WHERE job_id = ?
`, jobID.Int64).Scan(&total, &completed, &failed); err != nil {
		return err
	}
	percent := 0
	if total > 0 {
		percent = ((completed + failed) * 100) / total
	}

	if _, err := exec.Exec(`
UPDATE fetch_jobs
SET status = 'failed',
    total_steps = ?,
    completed_steps = ?,
    failed_steps = ?,
    progress_percent = ?,
    error_summary = CASE WHEN COALESCE(error_summary, '') = '' THEN '任务锁已过期' ELSE error_summary END,
    finished_at = COALESCE(finished_at, CURRENT_TIMESTAMP),
    updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND status = 'running'
`, total, completed, failed, percent, jobID.Int64); err != nil {
		return err
	}

	_, err = exec.Exec(`
UPDATE fetch_lock
SET job_id = NULL,
    locked_by_user_id = NULL,
    locked_by_username = NULL,
    locked_at = NULL,
    heartbeat_at = NULL,
    expires_at = NULL
WHERE id = 1 AND job_id = ?
`, jobID.Int64)
	return err
}

func ExpireStaleFetchLocks(db *sql.DB) error {
	return expireStaleFetchLocks(db)
}

func CreateFetchJob(db *sql.DB, userID int64, username string, programs []MiniProgram) (*FetchJob, error) {
	if len(programs) == 0 {
		return nil, errors.New("no enabled mini programs")
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}

	if err := expireStaleFetchLocks(tx); err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	var runningCount int
	if err := tx.QueryRow("SELECT COUNT(*) FROM fetch_jobs WHERE status = 'running'").Scan(&runningCount); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if runningCount > 0 {
		_ = tx.Rollback()
		return nil, errors.New("job already running")
	}

	totalSteps := len(programs) * len(jobstate.OrderedSteps)
	result, err := tx.Exec(`
INSERT INTO fetch_jobs (status, started_by_user_id, started_by_username, total_programs, total_steps, progress_percent)
VALUES ('running', ?, ?, ?, ?, 0)
`, userID, username, len(programs), totalSteps)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	jobID, err := result.LastInsertId()
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	for _, program := range programs {
		for _, step := range jobstate.OrderedSteps {
			if _, err := tx.Exec(`
INSERT INTO fetch_job_steps (job_id, program_id, program_name, app_id, step_type, status)
VALUES (?, ?, ?, ?, ?, 'pending')
`, jobID, program.ID, program.Name, program.AppID, string(step)); err != nil {
				_ = tx.Rollback()
				return nil, err
			}
		}
	}

	if _, err := tx.Exec(`
INSERT INTO fetch_lock (id, job_id, locked_by_user_id, locked_by_username, locked_at, heartbeat_at, expires_at)
VALUES (1, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?)
ON CONFLICT(id) DO UPDATE SET
    job_id = excluded.job_id,
    locked_by_user_id = excluded.locked_by_user_id,
    locked_by_username = excluded.locked_by_username,
    locked_at = CURRENT_TIMESTAMP,
    heartbeat_at = CURRENT_TIMESTAMP,
    expires_at = excluded.expires_at
`, jobID, userID, username, fetchJobLockExpiresAt(FetchJobLockTTL)); err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return GetFetchJobByID(db, jobID)
}

func GetLatestFetchJob(db *sql.DB) (*FetchJob, error) {
	return scanFetchJob(db.QueryRow(`
SELECT id, status, started_by_user_id, started_by_username, started_at, COALESCE(finished_at, ''),
       current_program_id, COALESCE(current_program_name, ''), COALESCE(current_step, ''),
       total_programs, total_steps, completed_steps, failed_steps, progress_percent,
       COALESCE(error_summary, ''), interrupt_requested, end_requested, created_at, updated_at
FROM fetch_jobs
ORDER BY id DESC
LIMIT 1
`))
}

func GetFetchJobByID(db *sql.DB, id int64) (*FetchJob, error) {
	return scanFetchJob(db.QueryRow(`
SELECT id, status, started_by_user_id, started_by_username, started_at, COALESCE(finished_at, ''),
       current_program_id, COALESCE(current_program_name, ''), COALESCE(current_step, ''),
       total_programs, total_steps, completed_steps, failed_steps, progress_percent,
       COALESCE(error_summary, ''), interrupt_requested, end_requested, created_at, updated_at
FROM fetch_jobs
WHERE id = ?
`, id))
}

func ListFetchJobs(db *sql.DB, userID int64, isAdmin bool, offset, limit int) ([]FetchJob, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var count int64
	var rows *sql.Rows
	var err error
	query := `
SELECT id, status, started_by_user_id, started_by_username, started_at, COALESCE(finished_at, ''),
       current_program_id, COALESCE(current_program_name, ''), COALESCE(current_step, ''),
       total_programs, total_steps, completed_steps, failed_steps, progress_percent,
       COALESCE(error_summary, ''), interrupt_requested, end_requested, created_at, updated_at
FROM fetch_jobs
`
	if isAdmin {
		err = db.QueryRow("SELECT COUNT(*) FROM fetch_jobs").Scan(&count)
		if err == nil {
			rows, err = db.Query(query+" ORDER BY id DESC LIMIT ? OFFSET ?", limit, offset)
		}
	} else {
		err = db.QueryRow("SELECT COUNT(*) FROM fetch_jobs WHERE started_by_user_id = ?", userID).Scan(&count)
		if err == nil {
			rows, err = db.Query(query+" WHERE started_by_user_id = ? ORDER BY id DESC LIMIT ? OFFSET ?", userID, limit, offset)
		}
	}
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	jobs := make([]FetchJob, 0)
	for rows.Next() {
		job, err := scanFetchJob(rows)
		if err != nil {
			return nil, 0, err
		}
		jobs = append(jobs, *job)
	}
	return jobs, count, rows.Err()
}

func ListFetchJobSteps(db *sql.DB, jobID int64) ([]FetchJobStep, error) {
	rows, err := db.Query(`
SELECT id, job_id, program_id, program_name, app_id, step_type, status,
       COALESCE(started_at, ''), COALESCE(finished_at, ''), record_count,
       COALESCE(error_message, ''), retry_count, created_at, updated_at
FROM fetch_job_steps
WHERE job_id = ?
ORDER BY id ASC
`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	steps := make([]FetchJobStep, 0)
	for rows.Next() {
		var step FetchJobStep
		var appID string
		if err := rows.Scan(
			&step.ID,
			&step.JobID,
			&step.ProgramID,
			&step.ProgramName,
			&appID,
			&step.StepType,
			&step.Status,
			&step.StartedAt,
			&step.FinishedAt,
			&step.RecordCount,
			&step.ErrorMessage,
			&step.RetryCount,
			&step.CreatedAt,
			&step.UpdatedAt,
		); err != nil {
			return nil, err
		}
		step.AppIDMasked = MaskAppID(appID)
		steps = append(steps, step)
	}
	return steps, rows.Err()
}

func MarkFetchJobStepRunning(db *sql.DB, stepID int64, programID int64, programName, stepType string) error {
	_, err := db.Exec(`
UPDATE fetch_job_steps
SET status = 'running',
    started_at = COALESCE(started_at, CURRENT_TIMESTAMP),
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, stepID)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
UPDATE fetch_jobs
SET current_program_id = ?,
    current_program_name = ?,
    current_step = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = (SELECT job_id FROM fetch_job_steps WHERE id = ?)
`, programID, programName, stepType, stepID)
	return err
}

func MarkFetchJobStepFinished(db *sql.DB, jobID int64, stepID int64, status string, recordCount int, errorMessage string) error {
	if status != string(jobstate.StepSuccess) && status != string(jobstate.StepFailed) && status != string(jobstate.StepSkipped) {
		return errors.New("invalid finished step status")
	}
	_, err := db.Exec(`
UPDATE fetch_job_steps
SET status = ?,
    finished_at = CURRENT_TIMESTAMP,
    record_count = ?,
    error_message = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, status, recordCount, errorMessage, stepID)
	if err != nil {
		return err
	}
	return RefreshFetchJobProgress(db, jobID)
}

func RefreshFetchJobProgress(db *sql.DB, jobID int64) error {
	var total, completed, failed int
	if err := db.QueryRow("SELECT COUNT(*) FROM fetch_job_steps WHERE job_id = ?", jobID).Scan(&total); err != nil {
		return err
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM fetch_job_steps WHERE job_id = ? AND status = 'success'", jobID).Scan(&completed); err != nil {
		return err
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM fetch_job_steps WHERE job_id = ? AND status = 'failed'", jobID).Scan(&failed); err != nil {
		return err
	}

	percent := 0
	if total > 0 {
		percent = ((completed + failed) * 100) / total
	}

	_, err := db.Exec(`
UPDATE fetch_jobs
SET total_steps = ?,
    completed_steps = ?,
    failed_steps = ?,
    progress_percent = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, total, completed, failed, percent, jobID)
	return err
}

func SetFetchJobTerminal(db *sql.DB, jobID int64, status string, errorSummary string) (*FetchJob, error) {
	if status != string(jobstate.JobCompleted) && status != string(jobstate.JobFailed) && status != string(jobstate.JobEnded) && status != string(jobstate.JobInterrupted) {
		return nil, errors.New("invalid terminal job status")
	}
	if err := RefreshFetchJobProgress(db, jobID); err != nil {
		return nil, err
	}
	if _, err := db.Exec(`
UPDATE fetch_jobs
SET status = ?,
    error_summary = ?,
    finished_at = COALESCE(finished_at, CURRENT_TIMESTAMP),
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, status, errorSummary, jobID); err != nil {
		return nil, err
	}
	_ = ReleaseFetchJobLock(db, jobID)
	return GetFetchJobByID(db, jobID)
}

func IsFetchJobRunning(db *sql.DB, jobID int64) (bool, error) {
	var status string
	var expiresAt string
	err := db.QueryRow(`
SELECT j.status, COALESCE(l.expires_at, '')
FROM fetch_jobs j
LEFT JOIN fetch_lock l ON l.id = 1 AND l.job_id = j.id
WHERE j.id = ?
`, jobID).Scan(&status, &expiresAt)
	if err != nil {
		return false, err
	}
	return status == string(jobstate.JobRunning) && expiresAt >= time.Now().UTC().Format(time.RFC3339), nil
}

func HeartbeatFetchJobLock(db *sql.DB, jobID int64, ttl time.Duration) error {
	result, err := db.Exec(`
UPDATE fetch_lock
SET heartbeat_at = CURRENT_TIMESTAMP,
    expires_at = ?
WHERE id = 1 AND job_id = ?
`, fetchJobLockExpiresAt(ttl), jobID)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("job lock is not held")
	}
	return nil
}

func InterruptFetchJob(db *sql.DB, jobID int64) (*FetchJob, error) {
	job, err := GetFetchJobByID(db, jobID)
	if err != nil {
		return nil, err
	}
	if job.Status != string(jobstate.JobRunning) {
		return nil, errors.New("job is not running")
	}
	if _, err := db.Exec(`
UPDATE fetch_jobs
SET status = 'interrupted',
    interrupt_requested = 1,
    finished_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, jobID); err != nil {
		return nil, err
	}
	_ = ReleaseFetchJobLock(db, jobID)
	return GetFetchJobByID(db, jobID)
}

func ResumeFetchJob(db *sql.DB, jobID int64, userID int64, username string) (*FetchJob, error) {
	job, err := GetFetchJobByID(db, jobID)
	if err != nil {
		return nil, err
	}
	if !jobstate.CanResume(jobstate.JobStatus(job.Status)) {
		return nil, errors.New("job is not resumable")
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	if err := expireStaleFetchLocks(tx); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	var runningCount int
	if err := tx.QueryRow("SELECT COUNT(*) FROM fetch_jobs WHERE status = 'running'").Scan(&runningCount); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if runningCount > 0 {
		_ = tx.Rollback()
		return nil, errors.New("job already running")
	}

	if _, err := tx.Exec(`
UPDATE fetch_job_steps
SET status = 'pending', started_at = NULL, finished_at = NULL, updated_at = CURRENT_TIMESTAMP
WHERE job_id = ? AND status = 'running'
`, jobID); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if _, err := tx.Exec(`
UPDATE fetch_jobs
SET status = 'running',
    interrupt_requested = 0,
    end_requested = 0,
    finished_at = NULL,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, jobID); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if _, err := tx.Exec(`
INSERT INTO fetch_lock (id, job_id, locked_by_user_id, locked_by_username, locked_at, heartbeat_at, expires_at)
VALUES (1, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?)
ON CONFLICT(id) DO UPDATE SET
    job_id = excluded.job_id,
    locked_by_user_id = excluded.locked_by_user_id,
    locked_by_username = excluded.locked_by_username,
    locked_at = CURRENT_TIMESTAMP,
    heartbeat_at = CURRENT_TIMESTAMP,
    expires_at = excluded.expires_at
`, jobID, userID, username, fetchJobLockExpiresAt(FetchJobLockTTL)); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return GetFetchJobByID(db, jobID)
}

func EndFetchJob(db *sql.DB, jobID int64) (*FetchJob, error) {
	job, err := GetFetchJobByID(db, jobID)
	if err != nil {
		return nil, err
	}
	if jobstate.IsTerminal(jobstate.JobStatus(job.Status)) {
		return nil, errors.New("job already terminal")
	}
	if _, err := db.Exec(`
UPDATE fetch_jobs
SET status = 'ended',
    end_requested = 1,
    finished_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, jobID); err != nil {
		return nil, err
	}
	_ = ReleaseFetchJobLock(db, jobID)
	return GetFetchJobByID(db, jobID)
}

func ReleaseFetchJobLock(db *sql.DB, jobID int64) error {
	_, err := db.Exec(`
UPDATE fetch_lock
SET job_id = NULL,
    locked_by_user_id = NULL,
    locked_by_username = NULL,
    locked_at = NULL,
    heartbeat_at = NULL,
    expires_at = NULL
WHERE id = 1 AND job_id = ?
`, jobID)
	return err
}

func ComputeJobPermissions(job *FetchJob, userID int64, role string) JobPermissions {
	if job == nil {
		return JobPermissions{CanStart: true}
	}

	isOwner := job.StartedByUserID == userID
	isAdmin := role == "admin"
	canOperate := isOwner || isAdmin
	status := jobstate.JobStatus(job.Status)
	isRunning := status == jobstate.JobRunning

	return JobPermissions{
		CanStart:     !isRunning,
		CanInterrupt: canOperate && isRunning,
		CanResume:    canOperate && jobstate.CanResume(status),
		CanEnd:       canOperate && isRunning,
	}
}

func CanOperateJob(job *FetchJob, userID int64, role string) bool {
	return role == "admin" || job.StartedByUserID == userID
}

func scanFetchJob(row rowScanner) (*FetchJob, error) {
	var job FetchJob
	var currentProgramID sql.NullInt64
	var interruptRequested, endRequested int
	err := row.Scan(
		&job.ID,
		&job.Status,
		&job.StartedByUserID,
		&job.StartedByUsername,
		&job.StartedAt,
		&job.FinishedAt,
		&currentProgramID,
		&job.CurrentProgramName,
		&job.CurrentStep,
		&job.TotalPrograms,
		&job.TotalSteps,
		&job.CompletedSteps,
		&job.FailedSteps,
		&job.ProgressPercent,
		&job.ErrorSummary,
		&interruptRequested,
		&endRequested,
		&job.CreatedAt,
		&job.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if currentProgramID.Valid {
		job.CurrentProgramID = &currentProgramID.Int64
	}
	job.InterruptRequested = interruptRequested == 1
	job.EndRequested = endRequested == 1
	return &job, nil
}

func JobStatusLabel(status string) string {
	switch strings.ToLower(status) {
	case "running":
		return "执行中"
	case "interrupted":
		return "已中断"
	case "failed":
		return "失败"
	case "ended":
		return "已结束"
	case "completed":
		return "已完成"
	default:
		return status
	}
}
