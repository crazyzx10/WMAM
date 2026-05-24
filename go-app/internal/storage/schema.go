package storage

type Migration struct {
	Version int
	Name    string
	SQL     string
}

var Migrations = []Migration{
	{
		Version: 1,
		Name:    "system_foundation",
		SQL: `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS system_meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('admin', 'user')),
    status TEXT NOT NULL CHECK (status IN ('active', 'disabled')) DEFAULT 'active',
    must_change_password INTEGER NOT NULL DEFAULT 0,
    password_changed_at DATETIME,
    last_login_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);
CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);

CREATE TABLE IF NOT EXISTS sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    remember INTEGER NOT NULL DEFAULT 0,
    expires_at DATETIME NOT NULL,
    revoked_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at DATETIME,
    user_agent TEXT,
    ip_address TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);

CREATE TABLE IF NOT EXISTS admin_recovery (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    recovery_hash TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    used_at DATETIME
);

CREATE TABLE IF NOT EXISTS system_config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    encrypted INTEGER NOT NULL DEFAULT 0,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS mini_programs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    app_id TEXT NOT NULL UNIQUE,
    app_secret_encrypted TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_mini_programs_enabled ON mini_programs(enabled);
`,
	},
	{
		Version: 2,
		Name:    "jobs_and_audit",
		SQL: `
CREATE TABLE IF NOT EXISTS fetch_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    status TEXT NOT NULL CHECK (status IN ('running', 'interrupted', 'failed', 'ended', 'completed')),
    started_by_user_id INTEGER NOT NULL,
    started_by_username TEXT NOT NULL,
    started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at DATETIME,
    current_program_id INTEGER,
    current_program_name TEXT,
    current_step TEXT,
    total_programs INTEGER NOT NULL DEFAULT 0,
    total_steps INTEGER NOT NULL DEFAULT 0,
    completed_steps INTEGER NOT NULL DEFAULT 0,
    failed_steps INTEGER NOT NULL DEFAULT 0,
    progress_percent INTEGER NOT NULL DEFAULT 0,
    error_summary TEXT,
    interrupt_requested INTEGER NOT NULL DEFAULT 0,
    end_requested INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (started_by_user_id) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_fetch_jobs_status ON fetch_jobs(status);
CREATE INDEX IF NOT EXISTS idx_fetch_jobs_started_by ON fetch_jobs(started_by_user_id);
CREATE INDEX IF NOT EXISTS idx_fetch_jobs_started_at ON fetch_jobs(started_at);

CREATE TABLE IF NOT EXISTS fetch_job_steps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id INTEGER NOT NULL,
    program_id INTEGER NOT NULL,
    program_name TEXT NOT NULL,
    app_id TEXT NOT NULL,
    step_type TEXT NOT NULL CHECK (step_type IN ('adunit_list', 'summary', 'detail', 'settlement')),
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'success', 'failed', 'skipped')),
    started_at DATETIME,
    finished_at DATETIME,
    record_count INTEGER NOT NULL DEFAULT 0,
    error_message TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (job_id) REFERENCES fetch_jobs(id)
);

CREATE INDEX IF NOT EXISTS idx_fetch_job_steps_job_id ON fetch_job_steps(job_id);
CREATE INDEX IF NOT EXISTS idx_fetch_job_steps_status ON fetch_job_steps(status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_fetch_job_steps_unique ON fetch_job_steps(job_id, program_id, step_type);

CREATE TABLE IF NOT EXISTS fetch_lock (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    job_id INTEGER,
    locked_by_user_id INTEGER,
    locked_by_username TEXT,
    locked_at DATETIME,
    heartbeat_at DATETIME,
    expires_at DATETIME
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER,
    username TEXT,
    action TEXT NOT NULL,
    target_type TEXT,
    target_id TEXT,
    description TEXT,
    result TEXT NOT NULL CHECK (result IN ('success', 'failed')) DEFAULT 'success',
    ip_address TEXT,
    user_agent TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id ON audit_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at);
`,
	},
}
