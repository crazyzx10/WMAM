# WMAM Multi-User Development Status

Date: 2026-05-25

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
   - Light/dark theme toggle
   - Collapsible desktop sidebar
   - Toast notifications for core operations
   - Job history detail modal

10. Runtime reliability
    - Embedded frontend routing works with `/api/*` routes
    - Fetch job lock heartbeat refreshes the active job lock
    - Expired fetch locks are marked failed and released before new jobs start

11. Deployment documentation
    - `WMAM-Deployment-Guide.md` documents local and server deployment
    - `README.md` is updated for the multi-user Web version

12. Release packaging
    - `scripts/build-release.ps1` builds Windows/Linux release directories
    - `scripts/build-release.sh` provides the same flow for Linux/macOS
    - Release output is ignored under `dist/`

13. Session security
    - Login now issues an HttpOnly `wmam_session` cookie with SameSite=Lax
    - "Remember password" keeps the server session for 30 days without storing the token in frontend storage
    - API requests and SSE streams use same-origin credentials
    - Bearer token support remains as a compatibility fallback

14. Frontend permission guard
    - Admin-only pages redirect ordinary users back to fetch execution even if they type the URL directly
    - Backend API permission enforcement remains the source of truth

15. Operation log pagination
    - Job history and audit log tabs now use the backend `total` values
    - Lightweight previous/next pagination keeps the log page usable as records grow

16. Sensitive system operation confirmation
    - Saving or restoring MySQL config now requires the current administrator password
    - Backup export and import require both the backup password and administrator password
    - The system page disables sensitive buttons until the administrator password is present

17. Login input spacing fix
    - Login input icons now reserve dedicated left padding
    - Password inputs reserve right padding for browser password visibility controls

18. Shell layout polish
    - Topbar title copy was removed from the app shell
    - Topbar and sidebar divider lines now share the same 64px border-box height model

## Remaining Stages

The previous eight small follow-up stages are now consolidated into four delivery stages. Each stage should still be implemented, tested, committed, and pushed as one complete unit.

19. Real fetch-path acceptance and data correctness
    - Verify the configured MySQL path with a real or representative database
    - Confirm the existing WeChat fetch logic writes all required ad data through the current job runner
    - Harden interrupt, resume, end, partial failure, lock release, and masked error behavior
    - Acceptance: admin and ordinary-user fetch flows pass, job summaries match database writes, and secrets do not leak into logs or API responses

20. Admin configuration and recovery hardening
    - Polish first-use setup guidance and admin recovery-code usability
    - Re-check MySQL config, mini-program config, user management, and destructive confirmations
    - Verify encrypted backup export/import overwrite behavior, restored accounts, and restored field encryption key handling
    - Acceptance: an administrator can recover from a wrong MySQL config and restore a backup without code-level intervention

21. UI polish and permission experience
    - Polish desktop spacing, alignment, empty states, loading states, error states, dark mode, tables, modals, and long-text handling
    - Keep ordinary-user screens focused on fetch execution and operation logs only
    - Re-check direct URL access, hidden admin-only actions, operation log pagination, filters, and detail views
    - Acceptance: administrator and ordinary-user workflows feel coherent, stable, and consistent in the browser

22. Release verification and handoff
    - Run full regression: frontend build, Go tests, release build, startup behavior, clean-data smoke test, and single-binary run check
    - Update README, deployment guide, and status docs with final run commands, default account notes, recovery notes, and known limits
    - Prepare the first usable release marker after final acceptance
    - Acceptance: the single binary can be copied to a server or local PC and used according to the documented deployment guide

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
