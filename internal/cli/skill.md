---
name: gdrive
description: Expert in Google Drive operations via the `gdrive` Go CLI and MCP server. Use for searching, uploading, downloading, sharing, managing permissions, tracking activity (changes, trash, history, revisions), and running the MCP server.
---

# gdrive Skill

Expert in driving the `gdrive` Go CLI and MCP server. Single source of truth for all Google Drive operations exposed by the binary; emitted by `gdrive skill` so it always matches the installed version.

## Core Capabilities

- Search files by name, query, or MIME type with shortcuts
- Upload files and folders with auto MIME detection and post-upload hooks
- Download files and folders with parallel transfers and timestamp preservation
- Copy, move, rename, delete files
- Share with users / groups / "anyone with the link"; list and remove permissions
- Get detailed file info including full Drive path, owners, dates
- Audit activity: changes, trash, full history (Drive Activity API), per-file revisions
- Run an MCP HTTP Streamable server exposing 21 Drive tools to AI agents

## When to Use This Skill

Use this skill when the user requests anything that maps to a Drive operation:

- "Search for files named X", "find X in my Drive", "where is X located"
- "Upload this file/folder to Drive", "back up this directory"
- "Download this file/folder", "sync this Drive folder locally"
- "Copy / move / rename / delete this file"
- "Share with X as editor", "make this public", "remove public access", "who has access"
- "List the contents of this folder"
- "What changed in my Drive recently", "what did I delete last week", "show me the full history", "show revisions of this file"
- "Start the MCP server", "run gdrive as an MCP endpoint"

**IMPORTANT:** When the user gives a document/file title without an ID (e.g., "open document XYZ", "share the v3 file"), ALWAYS search first with this skill to obtain the file ID before delegating to other skills (such as `google-docs-manager`).

## Binary Location

`gdrive` is a Go binary on `$PATH` (installed at `~/.local/bin/gdrive`). No runtime dependencies. Source: `~/Documents/Projects/perso/gdrive/`.

## Configuration

Configuration is resolved with priority **CLI flags > environment variables > defaults**.

| Setting | CLI flag | Environment variable | Default |
|---|---|---|---|
| Config directory | `--config-dir` | `GDRIVE_CONFIG_DIR` | `$HOME/.gdrive` |
| Credentials path | `--credentials` | `GDRIVE_CREDENTIALS_PATH` | `./credentials.json`, fallback `{config-dir}/credentials.json` |
| Token storage | (derived) | (derived) | `{config-dir}/token.json` |
| OTel trace file | (none) | `GDRIVE_TRACE_FILE` | unset (tracing disabled) |

Both `--config-dir` and `--credentials` are persistent flags — they work on every command.

```bash
# Use a non-default config directory for this invocation
gdrive --config-dir /tmp/gdrive-test search "report"

# Or via env var, persistent across invocations
export GDRIVE_CONFIG_DIR="$HOME/work/.gdrive"
gdrive search "report"
```

## Command Reference

```bash
# Search
gdrive search QUERY [--type TYPE[,TYPE]] [--max N]

# File operations
gdrive file download FILE [LOCAL_FOLDER] [--id] [--overwrite] [--format FMT]
gdrive file upload   LOCAL_FILE REMOTE_FOLDER [--id] [--mime MIME_TYPE] [--convert] [--run-after CMD]
gdrive file delete   FILE [--id]
gdrive file rename   FILE NEW_NAME [--id]
gdrive file move     FILE TARGET_FOLDER [--id]
gdrive file copy     FILE [NEW_NAME] [--parent FOLDER] [--id]
gdrive file info     FILE [--id]
gdrive file share    FILE EMAIL [--role ROLE] [--id] [--no-notify] [--message MSG]
gdrive file share-public      FILE [--role ROLE] [--id]
gdrive file permissions       FILE [--id]
gdrive file remove-permission FILE PERMISSION_ID [--id]
gdrive file remove-public     FILE [--id]

# Folder operations
gdrive folder create   REMOTE_FOLDER
gdrive folder list     FOLDER [--id]
gdrive folder upload   LOCAL_SRC REMOTE_FOLDER [--id] [--create] [--run-after CMD]
gdrive folder download FOLDER LOCAL_FOLDER [--id] [--overwrite] [--new-only] [--parallel N]

# Activity / audit
gdrive activity changes   [--max N]
gdrive activity deleted   [--days N] [--max N]
gdrive activity history   [--days N] [--max N]
gdrive activity revisions FILE [--id]

# MCP server
gdrive mcp [--port N] [--host HOST] [--base-url URL]
           [--credential-file PATH]
           [--secret-name NAME] [--secret-project ID]
           [--vault-addr URL] [--vault-token TOKEN] [--vault-secret-path PATH]

# Self-documentation
gdrive skill
```

## Path vs ID — the `--id` Flag

Every command that takes a `FILE` / `FOLDER` argument supports two modes:

- **Path mode (default):** human-readable Drive path like `My Drive/Documents/Q4.pdf`. Resolved recursively to an ID at execution time.
- **ID mode (`--id`):** the argument is treated as a raw Drive ID (e.g., `1iAinUz-fQB0G-juO3khaWxmxK-3EbPRq`).

```bash
# Path mode — no --id
gdrive file info "My Drive/Documents/Q4.pdf"

# ID mode — --id required
gdrive file info 1iAinUz-fQB0G-juO3khaWxmxK-3EbPRq --id
```

**Mistake to avoid:** passing an ID without `--id`. The CLI will treat the ID as a path and fail with a "not found" error.

For `move` / `copy --parent`: if BOTH source and destination are IDs, set `--id`. If both are paths, omit it. Mixed mode is not supported in a single call; use `file info` to convert one side first.

**When to prefer IDs:** files shared with you (no canonical path), files that move frequently, scripts that should not break on renames.
**When to prefer paths:** human-driven workflows, readability, ad-hoc commands.

## File Operations

### Upload — MIME auto-detection

`gdrive file upload` derives the Drive MIME type from the file extension. This matters because Office formats (`.pptx`, `.docx`, `.xlsx`, `.odt`, …) are ZIP archives internally; without explicit typing, Drive stores them as `application/zip`, shows a generic ZIP icon, and **prevents Google Slides / Docs / Sheets from opening the file natively**. Auto-detection tags them as the right OOXML / ODF MIME type so Workspace opens them correctly.

Recognized extensions include: `.pdf`, `.doc`/`.docx`/`.dotx`/`.docm`, `.xls`/`.xlsx`/`.xltx`/`.xlsm`, `.ppt`/`.pptx`/`.potx`/`.ppsx`/`.pptm`, `.odt`/`.ods`/`.odp`, `.epub`, `.rtf`, `.txt`/`.md`/`.csv`/`.tsv`/`.html`/`.json`/`.xml`/`.yaml`, common image / audio / video formats, and archive types. Unknown extensions fall back to `application/octet-stream` (Drive then runs server-side detection).

Override with `--mime` when needed:

```bash
# Force a specific MIME type
gdrive file upload deck.pptx 1abc123xyz --id \
  --mime application/vnd.openxmlformats-officedocument.presentationml.presentation

# Upload an extensionless file as plain text
gdrive file upload README "My Drive/Notes" --mime text/plain
```

### Upload — natural versioning

Drive keeps native version history. **Do NOT add `_v2`, `_v3` suffixes to filenames.** Upload with the same filename in the same target folder; `gdrive file upload` detects the existing file and creates a new revision. The CLI prints `Updating:` instead of `Uploading:` when this happens. Old revisions remain available under "Manage versions" in Drive UI.

```bash
# First call: creates the file
gdrive file upload report.pptx 1abc123xyz --id

# Same name later: stored as a new version of the same file
gdrive file upload report.pptx 1abc123xyz --id
```

### Upload — `--run-after` post-action

Both `file upload` and `folder upload` accept `--run-after CMD`. After a successful upload, the CLI runs `sh -c CMD` with the literal token `{}` substituted by the local source argument (`LOCAL_FILE` for files, `LOCAL_SRC` for folders).

A non-zero exit from the post-command bubbles up: `gdrive` exits non-zero even though the upload itself succeeded. Use this to clean up local copies, send notifications, etc.

```bash
# Trash the local file once uploaded
gdrive file upload ./toto.ogg Documents --run-after 'trash "{}"'

# Move a directory to an archive after backup
gdrive folder upload ./report-2024 "My Drive/Backups" --run-after 'mv "{}" ~/archive/'
```

The `{}` substitution is textual on the literal argument. Always quote `"{}"` in your command if the path may contain spaces.

### Upload — `--convert` to Google Workspace

`file upload --convert` asks Drive to convert the source file to the matching Google Workspace type during upload, based on the source extension:

| Source extensions | Target Workspace type |
|---|---|
| `.md`, `.txt`, `.html`, `.htm`, `.rtf`, `.doc`, `.docx`, `.odt` | Google Docs |
| `.csv`, `.tsv`, `.xls`, `.xlsx`, `.ods` | Google Sheets |
| `.ppt`, `.pptx`, `.odp` | Google Slides |

```bash
# Create a native Google Doc from a Markdown file
gdrive file upload ./spec.md "My Drive/Notes" --convert

# Create a native Google Sheet from a CSV
gdrive file upload ./data.csv "My Drive/Reports" --convert

# Re-upload to update the same Doc (Drive re-converts the new media)
gdrive file upload ./spec.md "My Drive/Notes" --convert
```

Rules:

- The file at the target name must either not exist or already be a Workspace document of the matching target type. Mismatch (e.g. `spec.md` exists as a plain text file) returns an error — rename or delete the existing file first.
- Unsupported extensions return an error listing the allowed ones.
- `--mime` still works alongside `--convert` (it overrides the *source* content type sent to Drive, e.g. forcing `text/markdown` for an extensionless markdown file).
- Without `--convert`, the file is uploaded as-is (no server-side conversion).

### Download — Google Workspace export

When downloading a Google Workspace file, `gdrive` automatically picks an export format. Defaults:

- Google Docs → `.pdf`
- Google Sheets → `.xlsx`
- Google Slides → `.pptx`
- Google Drawings → `.pdf`

Override with `--format FMT`. Supported values per source type:

- Docs: `md` (Markdown), `pdf` (default), `docx`, `txt`, `html`
- Sheets: `xlsx` (default), `csv`, `pdf`
- Slides: `pptx` (default), `pdf`
- Drawings: `pdf` (default), `png`, `jpg` / `jpeg`, `svg`

Forms, Sites, and Maps cannot be exported via the Drive API and will return `cannot export file type` — that is a Google limitation, not a gdrive one.

The local filename extension is adjusted to match the export format. Modification time from Drive is preserved on the local file via `os.Chtimes`.

```bash
gdrive file download "My Drive/Notes/Spec"   --format md     # → Spec.md
gdrive file download "My Drive/Reports/Q1"   --format csv    # → Q1.csv
gdrive file download 1abc --id --format docx                 # → <name>.docx
```

For non-Workspace (binary) files, `--format` is ignored: the file is downloaded byte-for-byte.

`folder download` always uses the per-type defaults; there is no per-file override flag for recursive downloads.

### Other file operations

```bash
gdrive file delete "My Drive/old-report.pdf"
gdrive file rename "Report.pdf" "Final Report.pdf"
gdrive file move   "Report.pdf" "My Drive/Archive"
gdrive file copy   "Report.pdf" "Report Copy.pdf"
gdrive file copy   "Report.pdf" --parent "My Drive/Archive"
gdrive file copy   1abc --parent 1xyz --id
gdrive file info   1abc --id   # full path, owners, size, type, dates
```

## Folder Operations

### Folder upload — `--create` flag

This flag changes the destination semantics:

- **Without `--create` (default, legacy):** contents of `LOCAL_SRC` are flattened directly into `REMOTE_FOLDER`. The `LOCAL_SRC` name is dropped. Subfolders inside `LOCAL_SRC` are preserved.
- **With `--create`:** a subfolder named `basename(LOCAL_SRC)` is created (or reused) inside `REMOTE_FOLDER`, then contents are uploaded into it. This is the intuitive "copy this directory there" behavior.

```bash
# Default: pours my_project/* into Documents/
gdrive folder upload ./my_project "My Drive/Documents"

# With --create: creates Documents/my_project/, pours contents there
gdrive folder upload ./my_project "My Drive/Documents" --create
```

**Recommendation:** use `--create` whenever the user says "upload this folder to X". The default exists for backwards compatibility.

### Folder download — `--parallel N`

`folder download` parallelises file downloads with a semaphore limited by `--parallel N` (default `5`, valid range `1`–`20`). Folders are processed sequentially (so directory structure is created before files), but file downloads inside a folder run concurrently.

```bash
# Conservative
gdrive folder download "My Drive/Project" ~/Downloads --parallel 3

# Aggressive (watch for API quota / rate limits)
gdrive folder download "My Drive/Project" ~/Downloads --parallel 15
```

Indicative timings (depends on file size, network, API quotas):

| Workload | sequential | `--parallel 5` | `--parallel 10` |
|---|---|---|---|
| 100 small files | ~50s | ~15s | ~10s |
| 10 × 1GB files | ~180s | ~45s | ~25s |
| Single file | ~1s | ~1s | ~1s |

### Folder download — `--new-only`

Skips files where the local copy is at least as recent as the Drive copy (mtime comparison). Useful for periodic syncs of a remote folder.

```bash
gdrive folder download "My Drive/Project" ~/sync/project --new-only --parallel 10
```

### Other folder operations

```bash
gdrive folder create "My Drive/Projects/2024/Q4"   # like mkdir -p
gdrive folder list   "My Drive/Documents"
gdrive folder list   1abc --id
```

## Search

```bash
gdrive search "Q4 Report"
gdrive search "budget 2024" --max 20
gdrive search Passeport --type image,pdf
gdrive search "My Project" --type folder
```

`--type` accepts comma-separated values mixing shortcuts and explicit MIME types.

**Type shortcuts:**

| shortcut | matches |
|---|---|
| `image` | all image MIME types |
| `audio` | audio files |
| `video` | video files |
| `prez` | Google Slides + PowerPoint |
| `doc` | Google Docs + Word |
| `spreadsheet` | Google Sheets + Excel |
| `txt` | text files |
| `pdf` | PDF |
| `folder` | folders only |

Output includes: title, ID, MIME type, modified date — copy the ID into follow-up commands.

## Sharing & Permissions

**Roles:** `reader`, `writer`, `commenter`, `owner` (transferring ownership requires the recipient to be in the same Workspace domain).

```bash
# Share with a user (notification email sent by default)
gdrive file share "Report.pdf" user@example.com --role writer
gdrive file share 1abc user@example.com --role writer --id

# Silent share (no notification)
gdrive file share "Report.pdf" user@example.com --role reader --no-notify

# Custom notification message
gdrive file share "Report.pdf" user@example.com --role reader --message "Review please"

# Share with anyone holding the link
gdrive file share-public "Report.pdf" --role reader
gdrive file share-public 1abc --role reader --id

# Inspect access
gdrive file permissions "Report.pdf"

# Revoke
gdrive file remove-public "Report.pdf"
gdrive file remove-permission "Report.pdf" PERMISSION_ID
```

`gdrive file permissions` prints each entry with its ID; pass that ID to `remove-permission` to revoke a specific user/group.

**Constraints:**
- Permissions can only be modified on files you own.
- Files shared *with* you are not editable for permissions.
- All operations support Shared Drives (the binary always sets `supportsAllDrives=true`).

## Activity & Audit

Four complementary commands; choose based on what you need to recover or audit.

### `activity changes` — recent change feed

Recent additions / modifications / removals in the user's Drive, ordered by change time. Best for "what's new since last time I looked".

```bash
gdrive activity changes
gdrive activity changes --max 200
```

### `activity deleted` — what's currently in the trash

Files in trash, with deletion time, size, and who deleted them. Suitable for recovery decisions before items are permanently purged. Filter by `--days` (default `7`).

```bash
gdrive activity deleted --days 30 --max 100
```

### `activity history` — full audit trail

Uses the **Drive Activity API v2**, which returns activities the basic Files API misses: permanent deletions, ownership changes, edits, moves, renames, restores, permission changes. Use this when investigating "what happened to this file".

```bash
gdrive activity history --days 14
gdrive activity history --days 30 --max 1000   # large traces
```

`--days` default `7`, `--max` default `100`. The API may cap historical retention; for older events, raise `--days` and `--max` together.

### `activity revisions` — per-file version log

Lists Drive's stored revisions for ONE file (modification time, size, author, keepForever flag, published flag). Useful before restoring an older version manually via the Drive UI.

```bash
gdrive activity revisions "My Drive/Reports/Q4.pdf"
gdrive activity revisions 1abc --id
```

Note: revision history may be incomplete for files with very long histories (Drive prunes old revisions unless `keepForever=true`).

## MCP Server

`gdrive mcp` starts an HTTP Streamable Model Context Protocol server exposing 21 Drive tools to AI agents.

### Local launch

```bash
# Minimum: local credentials file
gdrive mcp --port 8080 --credential-file credentials.json
```

### Flags and environment variables

Every flag below has an env-var fallback; flags win, env wins over default.

| Flag | Env var | Default | Purpose |
|---|---|---|---|
| `--port` | `PORT` | `8080` | HTTP listen port |
| `--host` | `HOST` | `0.0.0.0` | HTTP listen host |
| `--base-url` | `BASE_URL` | `http://localhost:PORT` | External URL advertised in OAuth metadata |
| `--credential-file` | `CREDENTIAL_FILE` | (none) | Local OAuth credentials JSON |
| `--secret-name` | `SECRET_NAME` | (none) | GCP Secret Manager secret name |
| `--secret-project` | `SECRET_PROJECT` | (none) | GCP project for Secret Manager |
| `--vault-addr` | `VAULT_ADDR` | (none) | HashiCorp Vault address |
| `--vault-token` | `VAULT_TOKEN` | (none) | Vault token |
| `--vault-secret-path` | `VAULT_SECRET_PATH` | (none) | Vault secret path |

Credential resolution order: **Secret Manager → Vault → local file**. Provide the inputs for whichever backend you intend; the others stay empty.

### Endpoints (default base `/`)

- `GET /health` — health probe
- `GET /.well-known/oauth-authorization-server` — RFC 8414 metadata
- `GET /.well-known/oauth-protected-resource` — RFC 9728 metadata
- `POST /register` — RFC 7591 dynamic client registration
- `GET /authorize` — OAuth2 authorization endpoint (proxies to Google, PKCE S256)
- `POST /token` — token endpoint
- `POST /mcp` — MCP HTTP Streamable endpoint (Bearer token required)

### Tools exposed (21)

12 read tools + 8 write tools + `ping`. All take Drive IDs (no path resolution server-side); transfers use signed URLs for binary data and direct content for text. Detailed tool reference: `.agent_docs/mcp-server.md` in the repository.

The `read content` tool exports Workspace files to text-friendly MIME types: Google Docs → **Markdown** (`text/markdown`), Google Sheets → CSV, Google Slides → plain text. Markdown preserves headings, lists, links, and tables, which is the LLM-friendly format.

### Deployment

`gdrive mcp` runs anywhere; production targets supported by the repo:

- **Cloud Run** (Terraform-managed): `make init-plan && make init-deploy` once, then `make plan && make deploy` per release. Custom domain `drive.mcp.scm-platform.org`. Full procedure in `.agent_docs/terraform.md`.
- **VPS via docker-compose + Vault**: `make deploy-vps` (latest tag) or `make deploy-vps VPS_TAG=v1.2.0`.

## Self-Documentation

```bash
gdrive skill           # prints this skill markdown to stdout
```

This is the **single source of truth** for AI consumers. The output is generated from content embedded in the binary; if a flag or behavior changes, the embedded content is updated in the same commit. Trust `gdrive skill` over any external skill file.

## Process → Command Mapping

Reusable workflows for common requests.

### Find and open a file referenced only by name

```bash
gdrive search "<title>" --type doc        # narrow by type if possible
# Pick the ID from the output, then:
gdrive file info <ID> --id                # confirm path / owner before acting
```

### Recover a recently deleted file

```bash
gdrive activity deleted --days 30
# Locate the entry, note its ID, restore via Drive UI (the CLI does not expose untrash today)
# OR if the file was permanently purged:
gdrive activity history --days 30 --max 500   # confirm permanent_delete event
gdrive activity revisions <ID> --id           # if the file shell still exists, list revisions
```

### Replace a presentation while preserving version history

```bash
# Same filename, same parent — Drive keeps both as one file with two revisions
gdrive file upload ./deck.pptx <PARENT_ID> --id
```

### Back up a local project to Drive

```bash
gdrive folder create "My Drive/Backups"
gdrive folder upload ./my-project "My Drive/Backups" --create
# With cleanup:
gdrive folder upload ./my-project "My Drive/Backups" --create \
  --run-after 'tar czf ~/.local-backup/{}.tgz "{}" && trash "{}"'
```

### Periodically sync a remote folder locally

```bash
gdrive folder download "My Drive/Project" ~/sync/project --new-only --parallel 10
```

### Make a file public for review, then revoke

```bash
gdrive file share-public "Report.pdf" --role reader
# share the link...
gdrive file remove-public "Report.pdf"
```

### Audit access on a file before sharing

```bash
gdrive file permissions <ID> --id
# Decide; remove anything stale:
gdrive file remove-permission <ID> <PERMISSION_ID> --id
```

### Audit "who deleted what" last week

```bash
gdrive activity deleted --days 7 --max 200    # currently in trash
gdrive activity history --days 7 --max 1000   # includes permanent deletes
```

### Stand up the MCP server locally for AI agent work

```bash
gdrive mcp --port 8080 --credential-file ~/.credentials/google_credentials.json
# In another terminal, point the agent at http://localhost:8080
```

## Best Practices

### `--id` discipline
- Prefer IDs in scripts and any context where the file may move.
- Always pair an ID argument with `--id`. The CLI will not heuristically detect.
- Get IDs from `gdrive search` output — it lists them next to each match.

### Search
- Use `--type` shortcuts to avoid drowning in noise: `--type pdf,doc`.
- `--max` defaults to 50 for `search`; raise it explicitly when needed.
- Partial titles work; Drive's search is fuzzy.

### Upload / download
- Trust auto MIME detection; only pass `--mime` to override.
- Never use `_v2`, `_v3` suffixes — rely on Drive's native versioning.
- For large folders, set `--parallel 10`–`15` and watch for 429s; back off if API quota errors appear.
- Use `--new-only` for repeat downloads of the same folder.

### Permissions
- List before mutating: `gdrive file permissions ...` so you know what exists.
- `share-public` makes the file accessible to anyone with the link — verify the user actually wants this.
- Permission IDs are stable; record them if you script revocations.

### Activity
- `changes` is for "what's new"; `history` is for "what happened" (incl. permanent deletes).
- Drive Activity API has retention limits — capture audits early when investigating an incident.

### MCP
- For local AI agent work, `--credential-file` is the simplest path.
- For production, prefer Secret Manager (Cloud Run) or Vault (VPS); avoid baking credentials into images.

## Authentication

- OAuth 2.0 against Google's auth server.
- CLI mode: browser-based consent on first run; token cached at `{config-dir}/token.json`; auto-refresh on expiry; if the refresh token is revoked, the CLI re-triggers the browser flow automatically.
- MCP mode: per-request Bearer token validated via the embedded RFC 8414/9728/7591 OAuth server with PKCE S256; tokens proxied to Google.

### First-time setup

```bash
mkdir -p ~/.gdrive
# Place credentials.json (Desktop OAuth client) at one of:
#   ./credentials.json          (current directory)
#   ~/.gdrive/credentials.json  (config directory)
gdrive search test    # opens browser for consent, saves ~/.gdrive/token.json
```

### Re-authenticate (token revoked or scope change)

```bash
rm ~/.gdrive/token.json
gdrive search test
```

## Troubleshooting

**"Google OAuth credentials not found"** — place `credentials.json` in `./` or `~/.gdrive/`, or pass `--credentials PATH`.

**"File not found" with an ID** — you forgot `--id`. The CLI is treating the ID as a path.

**Office file opens as ZIP / generic icon** — the file was uploaded before MIME auto-detection landed, or with `--mime application/zip`. Re-upload with the same name (no `--mime`) to fix the metadata; Drive keeps the version history.

**`folder upload` flattened my directory** — you didn't pass `--create`. Without it, the contents are poured directly into `REMOTE_FOLDER`. Re-run with `--create`.

**`activity history` returns nothing for old events** — Drive Activity API caps historical retention; older events may be unavailable. For recent incidents, run `gdrive activity history` quickly to capture the trail.

**Quota exceeded** — drop `--parallel`, add backoff between batches, check the GCP quotas page; for sustained high volume, request a quota increase.

**"Insufficient permissions" on share/remove-permission** — only file owners can change permissions. Ask the owner.

## Observability

Set `GDRIVE_TRACE_FILE=/path/to/trace.jsonl` before invocation to enable OpenTelemetry tracing. Spans are written as JSON lines (one per span). When the variable is unset, tracing is a no-op and adds no overhead.

```bash
GDRIVE_TRACE_FILE=$HOME/gdrive-trace.jsonl gdrive search "report"
GDRIVE_TRACE_FILE=/var/log/gdrive/trace.jsonl gdrive mcp --port 8080
```

Spans currently emitted:
- `auth.drive_service` — Drive API client construction (attribute `auth.mode` = `cli` | `mcp`)
- `auth.activity_service` — Drive Activity API client construction
- `mcp.request` — incoming MCP HTTP requests (attributes `http.method`, `http.path`)

Sensitive data (tokens, credentials, file content) is never recorded.

## Security Notes

- Credentials never leave the local machine in CLI mode; the binary does not log file content.
- All API calls use HTTPS.
- MCP mode validates a Bearer token on every request and emits proper `WWW-Authenticate` headers on auth failures (RFC 6750).
- Secret Manager / Vault integrations read credentials at server startup; rotate by restarting the server.
