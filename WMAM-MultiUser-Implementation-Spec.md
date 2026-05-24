# WMAM 多用户版本实现规格

**日期**：2026-05-24  
**状态**：开发前实现规格草案  
**上游文档**：[WMAM-MultiUser-Development-Plan.md](WMAM-MultiUser-Development-Plan.md)  
**目标**：把已确认的产品和 UI 方案转换为可开发、可验收的工程规格

---

## 1. 实现原则

第一版目标是轻量、稳定、可继续升级。

工程原则：

1. 保留并拆分现有微信广告拉取逻辑，不重写业务核心。
2. 后端使用 Go + Gin。
3. 前端使用 React + Vite + Tailwind CSS + shadcn/ui + Lucide + React Router。
4. WMAM 自身系统数据存储在云服务器 A 的 SQLite。
5. 微信广告数据写入外部 MySQL 服务器 B。
6. WMAM 启动后不主动连接 MySQL。
7. 不做 Docker。
8. 不做自动更新。
9. 不做移动端适配。
10. 不做定时任务。
11. 不做数据看板。
12. 第一版不预留租户/团队字段。

---

## 2. 目录结构

建议把现有 `go-app` 重构为以下结构：

```text
go-app/
  cmd/
    server/
      main.go

  internal/
    http/
      routes.go
      middleware/
        auth.go
        audit.go
        errors.go
      handlers/
        init.go
        auth.go
        users.go
        system.go
        programs.go
        jobs.go
        logs.go

    auth/
      service.go
      sessions.go
      passwords.go
      recovery.go

    users/
      service.go
      repository.go
      model.go

    systemconfig/
      service.go
      repository.go
      crypto.go
      mysql_test.go

    miniprograms/
      service.go
      repository.go
      model.go

    jobs/
      service.go
      runner.go
      state.go
      lock.go
      repository.go
      events.go

    fetcher/
      service.go
      wechat_api.go
      mysql_schema.go
      mysql_writer.go
      date_ranges.go
      token_cache.go

    audit/
      service.go
      repository.go

    backup/
      export.go
      import.go
      encrypt.go
      format.go

    storage/
      sqlite.go
      migrations.go

    crypto/
      fields.go
      random.go

  frontend/
    package.json
    vite.config.ts
    src/
      main.tsx
      app/
      pages/
      components/
      api/
      styles/

  config.yaml.example
```

拆分约束：

1. `internal/http` 只处理 HTTP 输入输出。
2. 业务规则放在 service 层。
3. 数据访问放在 repository 层。
4. `fetcher` 不依赖 Gin。
5. `jobs` 调度 `fetcher`，并负责状态、锁、事件推送。
6. 前端构建产物由 Go 服务托管。

---

## 3. 运行配置

运行级配置使用轻量 `config.yaml`。没有配置文件时使用默认值。

```yaml
server:
  host: "127.0.0.1"
  port: 28384

data:
  dir: "./data"
```

默认数据目录：

```text
data/
  wmam.db
  secret.key
  runtime/
```

规则：

1. `wmam.db` 是 SQLite 本地系统存储。
2. `secret.key` 是字段级加密主密钥。
3. `config.yaml` 不在网页端修改。
4. 业务配置通过系统配置页面修改。
5. 反向代理和 HTTPS 由外部 Nginx、Caddy、宝塔或 1Panel 处理。

---

## 4. SQLite 本地系统库

SQLite 存 WMAM 自身数据。表名使用英文，便于后续维护。

### 4.1 schema_migrations

```sql
CREATE TABLE schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

### 4.2 system_meta

存系统初始化状态和少量键值配置。

```sql
CREATE TABLE system_meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

建议 key：

1. `initialized`
2. `app_version`
3. `backup_format_version`

系统加密密钥不存入 SQLite 明文字段。运行时从 `secret.key` 加载，导出备份时把密钥纳入加密备份包。

### 4.3 users

```sql
CREATE TABLE users (
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

CREATE INDEX idx_users_role ON users(role);
CREATE INDEX idx_users_status ON users(status);
```

规则：

1. 只允许一个 `admin`。
2. 管理员不能禁用自己。
3. 管理员不能删除自己。
4. 管理员不能降级自己。
5. 普通用户首次登录必须修改密码。

### 4.4 sessions

保存登录会话和“记住密码”长期凭证。只保存 token 哈希。

```sql
CREATE TABLE sessions (
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

CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);
```

有效期：

1. 未勾选“记住密码”：24 小时。
2. 勾选“记住密码”：30 天。

失效场景：

1. 用户退出登录。
2. 用户修改密码。
3. 管理员重置用户密码。
4. 用户被禁用。
5. 导入系统配置后。

### 4.5 admin_recovery

保存管理员恢复码哈希。

```sql
CREATE TABLE admin_recovery (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    recovery_hash TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    used_at DATETIME
);
```

规则：

1. 系统只保存恢复码哈希。
2. 恢复码只能重置管理员密码。
3. 每次重置成功后旧恢复码失效，生成新恢复码。

### 4.6 system_config

保存业务系统配置。

```sql
CREATE TABLE system_config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    encrypted INTEGER NOT NULL DEFAULT 0,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

建议 key：

1. `mysql.host`
2. `mysql.port`
3. `mysql.database`
4. `mysql.username`
5. `mysql.password`，加密
6. `mysql.last_good_config`，加密 JSON

### 4.7 mini_programs

```sql
CREATE TABLE mini_programs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    app_id TEXT NOT NULL UNIQUE,
    app_secret_encrypted TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_mini_programs_enabled ON mini_programs(enabled);
```

规则：

1. `AppSecret` 字段级加密。
2. 前端不回显完整 `AppSecret`。
3. 编辑时留空保留旧值。

### 4.8 fetch_jobs

保存任务摘要。

```sql
CREATE TABLE fetch_jobs (
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

CREATE INDEX idx_fetch_jobs_status ON fetch_jobs(status);
CREATE INDEX idx_fetch_jobs_started_by ON fetch_jobs(started_by_user_id);
CREATE INDEX idx_fetch_jobs_started_at ON fetch_jobs(started_at);
```

### 4.9 fetch_job_steps

保存每个小程序每个步骤的状态，用于继续任务和历史详情。

```sql
CREATE TABLE fetch_job_steps (
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

CREATE INDEX idx_fetch_job_steps_job_id ON fetch_job_steps(job_id);
CREATE INDEX idx_fetch_job_steps_status ON fetch_job_steps(status);
CREATE UNIQUE INDEX idx_fetch_job_steps_unique ON fetch_job_steps(job_id, program_id, step_type);
```

步骤顺序：

1. `adunit_list`
2. `summary`
3. `detail`
4. `settlement`

### 4.10 fetch_lock

全局拉取锁。

```sql
CREATE TABLE fetch_lock (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    job_id INTEGER,
    locked_by_user_id INTEGER,
    locked_by_username TEXT,
    locked_at DATETIME,
    heartbeat_at DATETIME,
    expires_at DATETIME
);
```

规则：

1. 同一时间只允许一个运行任务。
2. 获取锁必须使用事务。
3. 任务完成、中断、结束后释放锁。
4. 如果锁过期，可由新任务接管。
5. 锁过期默认 30 分钟，但运行任务应定期 heartbeat 续期。

### 4.11 audit_logs

```sql
CREATE TABLE audit_logs (
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

CREATE INDEX idx_audit_logs_user_id ON audit_logs(user_id);
CREATE INDEX idx_audit_logs_action ON audit_logs(action);
CREATE INDEX idx_audit_logs_created_at ON audit_logs(created_at);
```

操作类型见本文第 12 节。

---

## 5. 外部 MySQL 广告数据表

第一版尽量沿用现有 `main.go` 中的 MySQL 表结构，降低迁移风险。

外部 MySQL 继续保存：

1. `mini_program`，可选同步，仅用于兼容现有数据结构。
2. `adunit_list`
3. `publisher_adpos_general`
4. `publisher_adunit_general`
5. `publisher_settlement`
6. `fetch_log`，可逐步废弃或仅保留兼容写入。

实现要求：

1. `fetcher` 在执行拉取前检查并创建 MySQL 表。
2. MySQL 连接失败不影响 WMAM 登录和系统配置页面。
3. MySQL 连接失败时，执行页实时日志展示错误。
4. MySQL 广告数据表不保存 WMAM 用户权限和系统配置。

后续可评估：

1. 是否将中文字段改为英文字段。
2. 是否提供迁移脚本。
3. 是否废弃 MySQL 中的 `mini_program` 和 `fetch_log`。

第一版不做这类破坏性迁移。

---

## 6. API 通用规范

### 6.1 响应格式

成功：

```json
{
  "success": true,
  "data": {}
}
```

失败：

```json
{
  "success": false,
  "error": {
    "code": "UNAUTHORIZED",
    "message": "请先登录"
  }
}
```

分页：

```json
{
  "success": true,
  "data": {
    "items": [],
    "page": 1,
    "pageSize": 20,
    "total": 100
  }
}
```

### 6.2 常用错误码

| code | 说明 |
|------|------|
| `BAD_REQUEST` | 请求参数错误 |
| `UNAUTHORIZED` | 未登录 |
| `FORBIDDEN` | 无权限 |
| `NOT_FOUND` | 资源不存在 |
| `CONFLICT` | 状态冲突 |
| `VALIDATION_ERROR` | 字段校验失败 |
| `MYSQL_CONNECTION_FAILED` | MySQL 连接失败 |
| `JOB_ALREADY_RUNNING` | 已有任务运行 |
| `JOB_NOT_RESUMABLE` | 任务不可继续 |
| `BACKUP_INVALID` | 备份文件无效 |
| `BACKUP_PASSWORD_INVALID` | 备份密码错误 |
| `INTERNAL_ERROR` | 服务内部错误 |

### 6.3 鉴权

使用 HttpOnly Cookie 保存 session token。

Cookie 建议：

1. `HttpOnly`
2. `SameSite=Lax`
3. 生产环境 HTTPS 下启用 `Secure`
4. Path 为 `/`

API 权限：

1. `/api/init/*`：未初始化时可访问。
2. `/api/auth/login`：未登录可访问。
3. `/api/auth/recover-admin/*`：未登录可访问。
4. `/api/app/*`：登录后可访问。
5. 管理员 API 需要 `role=admin`。

### 6.4 CSRF

因为使用 Cookie 鉴权，建议第一版加入轻量 CSRF 防护。

方案：

1. 登录后返回一个 CSRF token 给前端。
2. 前端对所有非 GET 请求加 `X-CSRF-Token`。
3. 后端校验 session 中的 CSRF token。
4. 初始化和登录接口可豁免或使用一次性 token。

如实现成本过高，至少保证：

1. Cookie 使用 `SameSite=Lax`。
2. 所有写操作只接受 JSON。
3. 校验 `Content-Type: application/json`。

---

## 7. 初始化 API

### 7.1 查询初始化状态

```text
GET /api/init/status
```

响应：

```json
{
  "success": true,
  "data": {
    "initialized": false
  }
}
```

### 7.2 测试初始化数据库连接

```text
POST /api/init/test-mysql
```

请求：

```json
{
  "host": "127.0.0.1",
  "port": 3306,
  "database": "wmam_ad",
  "username": "root",
  "password": "password"
}
```

响应：

```json
{
  "success": true,
  "data": {
    "message": "连接成功"
  }
}
```

### 7.3 创建新系统

```text
POST /api/init/create
```

请求：

```json
{
  "mysql": {
    "host": "127.0.0.1",
    "port": 3306,
    "database": "wmam_ad",
    "username": "root",
    "password": "password"
  },
  "admin": {
    "username": "admin",
    "password": "password"
  }
}
```

响应：

```json
{
  "success": true,
  "data": {
    "recoveryCode": "WMAM-XXXX-XXXX-XXXX"
  }
}
```

要求：

1. 只能在未初始化状态调用。
2. 创建唯一管理员。
3. 生成恢复码并只返回一次。
4. 保存恢复码哈希。
5. 生成或保存系统加密密钥。
6. 保存 MySQL 配置，密码字段加密。

### 7.4 从备份恢复初始化

```text
POST /api/init/restore
```

请求：

`multipart/form-data`

字段：

1. `file`
2. `backupPassword`

响应：

```json
{
  "success": true,
  "data": {
    "message": "恢复成功"
  }
}
```

要求：

1. 只能在未初始化状态调用。
2. 备份文件解密成功后覆盖本地系统存储。
3. 恢复系统加密密钥。
4. 恢复后所有 session 不应有效。

---

## 8. 认证 API

### 8.1 登录

```text
POST /api/auth/login
```

请求：

```json
{
  "username": "admin",
  "password": "password",
  "rememberPassword": true
}
```

响应：

```json
{
  "success": true,
  "data": {
    "user": {
      "id": 1,
      "username": "admin",
      "role": "admin",
      "mustChangePassword": false
    },
    "csrfToken": "..."
  }
}
```

规则：

1. 文案使用“记住密码”。
2. 技术上保存长期登录 session，不保存明文密码。
3. 用户被禁用时不可登录。
4. 普通用户首次登录后如果 `mustChangePassword=true`，前端只能进入修改密码页面。

### 8.2 当前用户

```text
GET /api/auth/me
```

响应：

```json
{
  "success": true,
  "data": {
    "user": {
      "id": 1,
      "username": "admin",
      "role": "admin",
      "mustChangePassword": false
    }
  }
}
```

### 8.3 退出登录

```text
POST /api/auth/logout
```

规则：

1. 撤销当前 session。
2. 清除 Cookie。

### 8.4 修改自己的密码

```text
POST /api/auth/change-password
```

请求：

```json
{
  "oldPassword": "old",
  "newPassword": "new"
}
```

规则：

1. 修改成功后清除该用户所有 session。
2. `must_change_password` 设为 0。
3. 写审计日志。

### 8.5 管理员恢复密码

```text
POST /api/auth/recover-admin
```

请求：

```json
{
  "recoveryCode": "WMAM-XXXX-XXXX-XXXX",
  "newPassword": "new-password"
}
```

响应：

```json
{
  "success": true,
  "data": {
    "newRecoveryCode": "WMAM-YYYY-YYYY-YYYY"
  }
}
```

规则：

1. 只允许重置唯一管理员密码。
2. 旧恢复码成功使用后失效。
3. 返回新恢复码，只显示一次。
4. 清除管理员所有 session。

---

## 9. 用户管理 API

均需管理员权限。

### 9.1 用户列表

```text
GET /api/users
```

响应：

```json
{
  "success": true,
  "data": {
    "users": [
      {
        "id": 2,
        "username": "user1",
        "role": "user",
        "status": "active",
        "mustChangePassword": true,
        "lastLoginAt": null,
        "createdAt": "2026-05-24T08:00:00Z"
      }
    ]
  }
}
```

### 9.2 创建普通用户

```text
POST /api/users
```

请求：

```json
{
  "username": "user1",
  "password": "initial-password"
}
```

规则：

1. 只能创建普通用户。
2. 不允许创建第二个管理员。
3. 创建后 `must_change_password=1`。

### 9.3 启用/禁用用户

```text
POST /api/users/:id/status
```

请求：

```json
{
  "status": "disabled"
}
```

规则：

1. 不能禁用唯一管理员。
2. 禁用用户后撤销该用户所有 session。

### 9.4 重置普通用户密码

```text
POST /api/users/:id/reset-password
```

请求：

```json
{
  "password": "new-initial-password"
}
```

规则：

1. 只能重置普通用户。
2. 重置后 `must_change_password=1`。
3. 撤销该用户所有 session。

---

## 10. 系统配置 API

均需管理员权限。

### 10.1 获取系统配置

```text
GET /api/system/config
```

响应：

```json
{
  "success": true,
  "data": {
    "mysql": {
      "host": "127.0.0.1",
      "port": 3306,
      "database": "wmam_ad",
      "username": "root",
      "passwordSet": true
    },
    "version": "0.1.0"
  }
}
```

规则：

1. 不返回 MySQL 密码。
2. 只返回 `passwordSet`。

### 10.2 测试 MySQL 连接

```text
POST /api/system/mysql/test
```

请求：

```json
{
  "host": "127.0.0.1",
  "port": 3306,
  "database": "wmam_ad",
  "username": "root",
  "password": "optional-new-password"
}
```

规则：

1. 如果 `password` 为空，使用已保存密码。
2. 测试成功不自动保存，除非调用保存接口。

### 10.3 保存 MySQL 配置

```text
PUT /api/system/mysql
```

请求：

```json
{
  "host": "127.0.0.1",
  "port": 3306,
  "database": "wmam_ad",
  "username": "root",
  "password": "",
  "adminPassword": "current-admin-password"
}
```

规则：

1. `password` 为空则保留旧密码。
2. 保存前必须测试连接成功。
3. 保存前把当前配置写入 `mysql.last_good_config`。
4. 需要管理员密码确认。
5. 写审计日志。

### 10.4 恢复上一份可用 MySQL 配置

```text
POST /api/system/mysql/restore-last-good
```

请求：

```json
{
  "adminPassword": "current-admin-password"
}
```

规则：

1. 需要二次确认。
2. 恢复后写审计日志。

### 10.5 导出系统配置

```text
POST /api/system/backup/export
```

请求：

```json
{
  "adminPassword": "current-admin-password",
  "backupPassword": "backup-password"
}
```

响应：

1. 返回加密备份文件下载。
2. 文件名建议：`wmam-backup-YYYYMMDD-HHMMSS.wmam`。

### 10.6 导入系统配置

```text
POST /api/system/backup/import
```

请求：

`multipart/form-data`

字段：

1. `file`
2. `backupPassword`
3. `adminPassword`

规则：

1. 需要管理员权限。
2. 需要管理员密码确认。
3. 导入后直接覆盖本地系统存储。
4. 导入后撤销所有 session。
5. 写审计日志。

---

## 11. 小程序配置 API

管理员可写，普通用户只在执行拉取页看到名称和状态。

### 11.1 小程序列表

```text
GET /api/programs
```

管理员响应包含更多字段：

```json
{
  "success": true,
  "data": {
    "programs": [
      {
        "id": 1,
        "name": "小程序 A",
        "appId": "wx1234****abcd",
        "appSecretSet": true,
        "enabled": true,
        "createdAt": "2026-05-24T08:00:00Z",
        "updatedAt": "2026-05-24T08:00:00Z"
      }
    ]
  }
}
```

规则：

1. 不返回完整 AppSecret。
2. AppID 可脱敏展示。

### 11.2 创建小程序

```text
POST /api/programs
```

请求：

```json
{
  "name": "小程序 A",
  "appId": "wx...",
  "appSecret": "secret",
  "enabled": true
}
```

规则：

1. 管理员权限。
2. `appSecret` 必填。
3. `appId` 唯一。
4. `appSecret` 加密保存。

### 11.3 更新小程序

```text
PUT /api/programs/:id
```

请求：

```json
{
  "name": "小程序 A",
  "appSecret": "",
  "enabled": true
}
```

规则：

1. 管理员权限。
2. `appSecret` 为空则保留旧值。
3. 输入新 `appSecret` 才更新。

### 11.4 删除小程序

```text
DELETE /api/programs/:id
```

规则：

1. 管理员权限。
2. 需要确认弹窗。
3. 删除不影响已经写入 MySQL 的历史广告数据。

### 11.5 启用/禁用小程序

```text
POST /api/programs/:id/status
```

请求：

```json
{
  "enabled": false
}
```

---

## 12. 任务 API

### 12.1 当前任务状态

```text
GET /api/jobs/current
```

响应：

```json
{
  "success": true,
  "data": {
    "job": {
      "id": 10,
      "status": "running",
      "startedByUserId": 1,
      "startedByUsername": "admin",
      "currentProgramName": "小程序 B",
      "currentStep": "summary",
      "progressPercent": 65,
      "completedSteps": 8,
      "failedSteps": 1,
      "totalSteps": 20
    },
    "permissions": {
      "canStart": false,
      "canInterrupt": true,
      "canResume": false,
      "canEnd": true
    },
    "steps": []
  }
}
```

权限规则：

1. 普通用户如果当前任务不是自己发起，只能看到有限整体状态。
2. 管理员可看到完整状态。

### 12.2 开始任务

```text
POST /api/jobs/start
```

规则：

1. 管理员和普通用户均可调用。
2. 如果已有任务运行，返回 `JOB_ALREADY_RUNNING`。
3. 拉取全部已启用小程序。
4. 创建 `fetch_jobs` 和 `fetch_job_steps`。
5. 获取全局锁。
6. 启动后台任务。

### 12.3 中断任务

```text
POST /api/jobs/:id/interrupt
```

规则：

1. 普通用户只能中断自己任务。
2. 管理员可中断任何任务。
3. 状态变为 `interrupted`。
4. 释放锁。
5. 保留已完成步骤。

### 12.4 继续任务

```text
POST /api/jobs/:id/resume
```

规则：

1. 仅 `interrupted` 和 `failed` 可继续。
2. 普通用户只能继续自己任务。
3. 管理员可继续任何任务。
4. 只重试失败或未完成步骤。
5. 已成功步骤不重复执行。

### 12.5 结束任务

```text
POST /api/jobs/:id/end
```

规则：

1. 普通用户只能结束自己任务。
2. 管理员可结束任何任务。
3. 状态变为 `ended`。
4. 不允许继续。
5. 释放锁。
6. 已入库数据保留。

### 12.6 任务事件流

```text
GET /api/jobs/:id/events
```

使用 SSE。

事件类型：

```json
{"type":"log","level":"info","time":"10:30:00","message":"开始拉取"}
{"type":"progress","percent":65,"completedSteps":8,"failedSteps":1}
{"type":"step","programId":1,"stepType":"summary","status":"success"}
{"type":"complete","status":"completed"}
```

规则：

1. 执行详细日志只通过 SSE 推送到当前页面。
2. 不保存详细日志到 SQLite。
3. 页面刷新后详细日志消失。
4. 任务结束后当前页面仍保留已有日志。

### 12.7 历史任务列表

```text
GET /api/jobs
```

查询参数：

1. `page`
2. `pageSize`
3. `status`
4. `startDate`
5. `endDate`
6. `userId`，管理员可用

普通用户只返回自己任务。

### 12.8 任务详情

```text
GET /api/jobs/:id
```

返回：

1. 任务摘要。
2. 小程序步骤摘要。
3. 失败摘要。
4. 不返回执行详细日志。

---

## 13. 审计日志 API

### 13.1 审计日志列表

```text
GET /api/audit-logs
```

查询参数：

1. `page`
2. `pageSize`
3. `actionGroup`
4. `startDate`
5. `endDate`
6. `userId`，管理员可用

普通用户只返回自己的审计日志。

### 13.2 审计操作类型

建议操作类型：

| action | 说明 |
|--------|------|
| `LOGIN` | 登录 |
| `LOGOUT` | 登出 |
| `PASSWORD_CHANGE` | 修改密码 |
| `ADMIN_RECOVERY_RESET` | 管理员恢复码重置密码 |
| `USER_CREATE` | 创建用户 |
| `USER_DISABLE` | 禁用用户 |
| `USER_ENABLE` | 启用用户 |
| `USER_PASSWORD_RESET` | 重置用户密码 |
| `PROGRAM_CREATE` | 创建小程序 |
| `PROGRAM_UPDATE` | 更新小程序 |
| `PROGRAM_DELETE` | 删除小程序 |
| `PROGRAM_ENABLE` | 启用小程序 |
| `PROGRAM_DISABLE` | 禁用小程序 |
| `SYSTEM_MYSQL_TEST` | 测试 MySQL |
| `SYSTEM_MYSQL_UPDATE` | 修改 MySQL 配置 |
| `SYSTEM_MYSQL_RESTORE` | 恢复上一份 MySQL 配置 |
| `BACKUP_EXPORT` | 导出系统配置 |
| `BACKUP_IMPORT` | 导入系统配置 |
| `JOB_START` | 开始拉取 |
| `JOB_INTERRUPT` | 中断拉取 |
| `JOB_RESUME` | 继续拉取 |
| `JOB_END` | 结束拉取 |
| `JOB_COMPLETE` | 任务完成 |
| `JOB_FAILED` | 任务失败 |

---

## 14. 任务状态机

### 14.1 状态流转

```text
start
  ↓
running
  ├─ all steps success ─────────────→ completed
  ├─ some steps failed after finish ─→ failed
  ├─ interrupt requested ───────────→ interrupted
  ├─ end requested ─────────────────→ ended
  └─ fatal error ───────────────────→ failed

interrupted
  └─ resume ────────────────────────→ running

failed
  └─ resume ────────────────────────→ running

ended
  └─ no further transition

completed
  └─ no further transition
```

### 14.2 按钮可用性

| 任务状态 | 开始 | 中断 | 继续 | 结束 |
|----------|------|------|------|------|
| 无任务/空闲 | 可用 | 禁用 | 禁用 | 禁用 |
| running 且自己发起 | 禁用 | 可用 | 禁用 | 可用 |
| running 且他人发起，普通用户 | 禁用 | 禁用 | 禁用 | 禁用 |
| running 且他人发起，管理员 | 禁用 | 可用 | 禁用 | 可用 |
| interrupted 且自己发起 | 可用 | 禁用 | 可用 | 禁用 |
| interrupted 且他人发起，普通用户 | 禁用 | 禁用 | 禁用 | 禁用 |
| interrupted 且他人发起，管理员 | 可用 | 禁用 | 可用 | 禁用 |
| failed 且自己发起 | 可用 | 禁用 | 可用 | 禁用 |
| failed 且他人发起，管理员 | 可用 | 禁用 | 可用 | 禁用 |
| ended | 可用 | 禁用 | 禁用 | 禁用 |
| completed | 可用 | 禁用 | 禁用 | 禁用 |

说明：

1. `开始` 会创建新任务。
2. `继续` 会继续已有 `interrupted` 或 `failed` 任务。
3. 已结束任务不可继续。
4. 已完成任务不可继续。

### 14.3 步骤状态

步骤状态：

1. `pending`
2. `running`
3. `success`
4. `failed`
5. `skipped`

继续任务时：

1. `success` 不重复执行。
2. `failed` 重试。
3. `pending` 执行。
4. `running` 从上次中断时重置为 `pending` 或 `failed` 后再执行。

---

## 15. 前端页面结构

### 15.1 路由

```text
/init
/login
/recover-admin
/change-password
/app/fetch
/app/logs
/app/programs
/app/system
/app/users
```

路由守卫：

1. 未初始化：跳转 `/init`。
2. 未登录：跳转 `/login`。
3. `mustChangePassword=true`：只能访问 `/change-password`。
4. 普通用户访问管理员路由：403 或跳转 `/app/fetch`。

### 15.2 主要组件

基础组件优先来自 shadcn/ui：

1. `Button`
2. `Input`
3. `Dialog`
4. `Tabs`
5. `Table`
6. `DropdownMenu`
7. `Tooltip`
8. `Toast`
9. `Switch`
10. `Badge`
11. `Alert`
12. `Progress`

业务组件：

1. `AppLayout`
2. `Sidebar`
3. `Topbar`
4. `ThemeToggle`
5. `ProtectedRoute`
6. `AdminRoute`
7. `FetchTaskSummaryCard`
8. `ProgramStepTable`
9. `RealtimeLogPanel`
10. `TaskDetailDialog`
11. `ConfirmDialog`
12. `SystemBackupPanel`
13. `MysqlConfigForm`

### 15.3 页面验收

登录页：

1. 能登录。
2. 支持“记住密码”。
3. 错误显示在登录卡片内。
4. 支持主题切换。

执行拉取页：

1. 当前任务卡片显示正确。
2. 四个按钮固定显示。
3. 按任务状态禁用按钮。
4. 小程序步骤列表显示 4 个步骤状态。
5. 实时日志固定高度滚动。
6. 页面刷新后实时日志消失。

系统配置页：

1. 密码不回显。
2. 留空保留旧密码。
3. 测试连接成功后才能保存。
4. 支持恢复上一份可用配置。
5. 支持导入导出系统配置。

操作日志页：

1. 任务记录和审计日志使用 Tab。
2. 分页每页 20 条。
3. 普通用户只能看到自己的记录。
4. 管理员可以筛选用户。

---

## 16. 安全规格

### 16.1 密码

1. 用户密码使用 bcrypt。
2. 不保存明文密码。
3. 重置密码后撤销所有 session。
4. 修改密码后撤销所有 session。

### 16.2 敏感字段加密

字段级加密对象：

1. MySQL 密码。
2. 小程序 AppSecret。

建议算法：

1. AES-256-GCM。
2. 每次加密使用随机 nonce。
3. 密文格式包含版本、nonce、ciphertext。

### 16.3 备份文件

备份文件包含：

1. SQLite 系统配置数据。
2. 密码哈希。
3. 系统加密密钥。
4. 小程序配置密文。
5. MySQL 配置密文。
6. 任务摘要。
7. 审计日志。

备份文件不包含：

1. 明文用户密码。
2. 登录 session。
3. 实时执行详细日志。
4. 外部 MySQL 广告数据。

备份文件加密：

1. 管理员设置备份文件密码。
2. 使用 KDF 从备份密码派生密钥，建议 Argon2id 或 scrypt。
3. 使用 AES-256-GCM 加密备份包。
4. 备份密码无法找回。

### 16.4 日志脱敏

任何日志中不得出现：

1. AppSecret。
2. access_token。
3. MySQL 密码。
4. 带敏感参数的完整 URL。
5. 用户密码。
6. 备份文件密码。

---

## 17. 从现有代码迁移

现有代码可保留的核心逻辑：

1. 微信 access_token 获取和缓存。
2. 微信广告 API 调用。
3. 分页拉取。
4. 月份范围拆分。
5. 广告位清单保存。
6. 汇总数据保存。
7. 细分数据保存。
8. 结算数据保存。
9. MySQL 建表逻辑。
10. 增量日期判断逻辑。

需要改造：

1. 配置来源从 `.env` 改为 SQLite 系统配置。
2. 小程序列表来源从 `.env` 改为 SQLite `mini_programs`。
3. HTTP 接口从 `net/http` 改为 Gin。
4. 前端从单 HTML 改为 React。
5. 执行拉取从单请求流式执行改为 job runner。
6. 日志推送使用 SSE event。
7. 中断/继续/结束通过 job 状态控制。

旧 `.env`：

1. 第一版可不做自动迁移。
2. 可在初始化向导手动填写数据库配置。
3. 小程序配置由管理员在页面手动添加。

---

## 18. 测试计划

### 18.1 权限测试

1. 普通用户不能访问小程序配置。
2. 普通用户不能访问系统配置。
3. 普通用户不能访问用户管理。
4. 普通用户不能导入导出配置。
5. 普通用户不能操作别人任务。
6. 管理员能操作任何任务。
7. 禁用用户后无法登录。

### 18.2 初始化测试

1. 未初始化时进入 `/init`。
2. 初始化创建管理员成功。
3. 恢复码只显示一次。
4. 未保存恢复码不能完成初始化。
5. 可从备份恢复初始化。

### 18.3 登录测试

1. 正确账号密码登录成功。
2. 错误密码提示错误。
3. 记住密码 30 天有效。
4. 退出后 session 失效。
5. 修改密码后 session 失效。
6. 普通用户首次登录必须修改密码。

### 18.4 系统配置测试

1. MySQL 连接成功提示成功。
2. MySQL 连接失败提示失败。
3. 密码留空保存保留旧值。
4. 保存前必须测试连接。
5. 恢复上一份可用配置成功。

### 18.5 拉取任务测试

1. 没有任务时可以开始。
2. 已有任务运行时不能重复开始。
3. 普通用户只能中断自己任务。
4. 管理员可以中断别人任务。
5. 中断后可继续。
6. 失败后可继续。
7. 结束后不可继续。
8. 单步骤失败后继续后续步骤。
9. 任务部分失败最终状态为 `failed`。
10. 页面刷新后详细日志消失。

### 18.6 备份测试

1. 导出需要管理员密码。
2. 导出需要备份密码。
3. 备份文件不能明文读取敏感数据。
4. 正确备份密码可导入。
5. 错误备份密码不可导入。
6. 导入后所有 session 失效。

### 18.7 UI 测试

1. 浅色模式显示正常。
2. 暗色模式显示正常。
3. 侧边栏折叠正常。
4. 1080px 内容宽度生效。
5. 执行页三张卡片布局正常。
6. Toast、Dialog、Tabs、Table 状态正常。

---

## 19. 验收标准

第一版完成标准：

1. 能初始化新系统。
2. 能创建唯一管理员。
3. 能登录和退出。
4. 能创建普通用户。
5. 普通用户首次登录必须改密码。
6. 管理员能配置 MySQL。
7. 管理员能配置小程序。
8. 普通用户只能看到执行拉取和操作日志。
9. 能手动执行拉取全部启用小程序。
10. 能中断自己的任务。
11. 能继续自己的中断或失败任务。
12. 能结束自己的任务，且结束后不可继续。
13. 管理员能操作任何任务。
14. 实时日志能显示，且不落库。
15. 历史任务摘要能查看。
16. 审计日志能查看。
17. 系统配置能加密导出。
18. 系统配置能导入覆盖。
19. 暗色模式可用。
20. 可通过反向代理部署到 HTTPS。

