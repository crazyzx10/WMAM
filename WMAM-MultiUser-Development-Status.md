# WMAM Multi-User Development Status

Date: 2026-05-24

This branch implements the first multi-user Web application foundation for WMAM.

## Completed Stages

1. Local system storage
   - SQLite system database under `go-app/data/wmam-system.db`
   - Field encryption key under `go-app/data/secret.key`
   - Runtime config from `go-app/config.yaml`

2. Authentication and accounts
   - Default administrator: `admin / admin123`
   - JWT plus local session validation
   - 30-day "remember password" login credential
   - Change password flow
   - Admin recovery code flow
   - Unique administrator protection
   - Ordinary user creation, disable, enable, delete, and password reset

3. Permissions
   - Ordinary users only see fetch execution and logs
   - Admin-only APIs protect users, system config, mini-program config, import, and export
   - Disabled users and reset-password users lose old sessions

4. System configuration
   - MySQL connection config stored in local SQLite
   - MySQL password encrypted with AES-256-GCM
   - Previous usable MySQL config can be restored
   - WMAM startup does not connect to MySQL

5. Mini-program configuration
   - Mini-program config stored in local SQLite
   - AppSecret encrypted with AES-256-GCM
   - AppSecret is never returned to the frontend
   - Admin can create, update, enable, disable, and delete programs

6. Fetch jobs
   - Fetch job summaries stored in SQLite
   - Per-program and per-step status stored in SQLite
   - Start, interrupt, resume, and end APIs
   - Ordinary users can operate only their own jobs
   - Admin can operate any job
   - Background runner invokes the existing WeChat/MySQL fetch functions

7. Logs
   - Audit logs stored in SQLite
   - Job history and audit logs available from the Operation Logs page
   - Real-time detailed execution logs are kept in the current page only

8. Backup
   - Encrypted backup export
   - Encrypted backup import and overwrite
   - Backup includes local system data and field encryption key
   - Backup excludes sessions and realtime execution logs

9. Frontend
   - React + Vite + Tailwind CSS + Lucide
   - Built frontend bundle is emitted to `go-app/frontend`
   - Go embeds and serves the built frontend

## Verification

Run from `go-app/web`:

```bash
npm install
npm run build
```

Run from `go-app`:

```bash
go test ./...
go run .
```

The application listens on `http://127.0.0.1:28384` by default.

## Notes

- MySQL is only connected when an admin tests/saves MySQL config or when a fetch job runs.
- The current runner updates job summaries and step status. Detailed realtime log persistence is intentionally not implemented.
- Docker deployment is intentionally not included in the first version.
