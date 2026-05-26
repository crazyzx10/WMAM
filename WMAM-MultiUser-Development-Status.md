# WMAM Multi-User Development Status

Date: 2026-05-26

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
    - Backup export and import require both the backup password and administrator password
    - Recovery-code rotation requires the current administrator password
    - MySQL config save and restore are available to authenticated administrators from the System page

17. Login input spacing fix
    - Login input icons now reserve dedicated left padding
    - Password inputs reserve right padding for browser password visibility controls

18. Shell layout polish
    - Topbar title copy was removed from the app shell
    - Topbar and sidebar divider lines now share the same 64px border-box height model

19. Real fetch-path acceptance and data correctness
    - Multi-user fetch jobs now use the correct Chinese date column for incremental summary and detail pulls
    - Ad-unit list steps now report the processed record count back to the job summary
    - Summary and detail steps now report partial range/save failures as failed steps while keeping successful writes
    - Failed steps are cleaned before retry so resume can re-run failed or unfinished work accurately
    - Ending a job marks unfinished steps as skipped and moves progress to a coherent terminal state
    - Runtime error redaction now masks known secrets and sensitive query parameters before they reach job summaries or live logs

20. Admin configuration and recovery hardening
    - Admins can generate a new recovery code from the System page after confirming the current admin password
    - Recovery codes can be copied or downloaded immediately after generation or admin-password recovery
    - MySQL restore availability is exposed to the frontend so the restore button is disabled until a last-good config exists
    - MySQL connection test and save errors now redact passwords before returning messages to the browser
    - Backup import clears restored sessions, clears restored fetch locks, and marks imported running jobs as failed
    - Backup import now forces the frontend back to login after the local system store is overwritten

21. UI polish and permission experience
    - Shared page header, status message, and table shell components keep admin pages visually consistent and easier to replace later
    - Fetch execution now has initial loading feedback, stable step markers, and long realtime log lines wrap safely
    - Operation logs now have refresh feedback, current-page filters, horizontally scrollable tables, and detail-modal long-text handling
    - Mini-program and user management tables now handle long names and IDs better, with confirmation before disabling a program or user
    - System, mini-program, and user forms now use responsive grid behavior and consistent success/error feedback

22. Release verification and handoff
    - README, deployment guide, and status docs document final run commands, default account notes, recovery notes, backup notes, and known limits
    - Full regression includes frontend build, Go tests, release build, startup behavior, clean-data smoke test, and single-binary run check
    - Release packaging copies the final README and deployment guide into the release directory
    - Acceptance: the single binary can be copied to a server or local PC and used according to the documented deployment guide

## Remaining Stages

None. The multi-user Web first version is feature-complete for the agreed lightweight delivery scope.

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

Build release directories from the repository root:

```powershell
.\scripts\build-release.ps1 -Target current
.\scripts\build-release.ps1 -Target linux-amd64
```

Linux/macOS:

```bash
./scripts/build-release.sh current
./scripts/build-release.sh linux-amd64
```

The Windows release binary is `dist/wmam-windows-amd64/wmam-server.exe`.

## Notes

- MySQL is only connected when an admin tests/saves MySQL config or when a fetch job runs.
- The current runner updates job summaries and step status. Detailed realtime log persistence is intentionally not implemented.
- Docker deployment is intentionally not included in the first version.
- Default administrator is `admin / admin123`; first login should change the password immediately.
- Admin recovery code is printed once during first startup and can later be regenerated from the System page.
