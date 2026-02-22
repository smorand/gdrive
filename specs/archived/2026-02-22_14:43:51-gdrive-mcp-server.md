# gdrive MCP Server -- Specification Document

> Generated on: 2026-02-22
> Version: 1.0
> Status: Draft

## 1. Executive Summary

This specification defines the addition of an MCP (Model Context Protocol) HTTP Streamable server to the existing `gdrive` CLI tool. The MCP server will expose all Google Drive operations as MCP tools, enabling AI agents (Claude, Cursor, etc.) to programmatically interact with Google Drive via a standardized protocol.

The server replicates the OAuth2 and infrastructure patterns from the `google-contacts` project but as a fully independent codebase with its own GCP project, OAuth credentials, Firestore database, and Secret Manager secrets. It will be deployed on Cloud Run at `drive.mcp.scm-platform.org` with Terraform-managed infrastructure.

Key design decisions:
- **ID-only tool parameters**: MCP tools accept Google Drive file/folder IDs exclusively (no path resolution); agents use search/list tools to discover IDs
- **Signed URLs for file transfers**: Download, export, and upload operations return authenticated URLs rather than streaming file content through the server
- **Soft delete only**: Delete operations move files to trash (no permanent delete in v1)
- **Consolidated permission tools**: Two tools (list + update) cover all permission operations

## 2. Scope

### 2.1 In Scope

- MCP HTTP Streamable server as a new `gdrive mcp` subcommand
- All existing Drive operations exposed as MCP tools (20 tools total)
- OAuth 2.1 authorization server (RFC 8414, RFC 9728, RFC 7591, PKCE S256)
- Context-based token injection for per-user isolated requests
- Terraform infrastructure (Cloud Run, Firestore, Secret Manager, Cloud DNS, Artifact Registry)
- Config-driven deployment (`config.yaml` as single source of truth)
- Custom domain: `drive.mcp.scm-platform.org`
- Structured logging with `slog` (JSON in production, text locally)
- Health check endpoint
- Suggested implementation phases

### 2.2 Out of Scope (Non-Goals)

- Shared code or secrets with `google-contacts` project
- stdio or SSE transport (HTTP Streamable only)
- Permanent file deletion (soft delete / trash only in v1)
- Human-readable path resolution in MCP tools (ID-only)
- File content streaming through the MCP server (signed URLs instead)
- Interactive confirmations (agents are autonomous callers)
- Web UI or admin dashboard
- Rate limiting on OAuth endpoints (v1)
- IP allowlisting

## 3. User Personas & Actors

### AI Agent (Primary)
An LLM-based AI assistant (Claude, Cursor, ChatGPT, etc.) that connects to the MCP server via HTTP Streamable transport. The agent authenticates with an OAuth2 Bearer token and calls MCP tools to perform Google Drive operations on behalf of a human user.

### Human User (Indirect)
The person whose Google Drive is being accessed. They authenticate once via the OAuth2 browser flow, which stores their refresh token in Firestore. They do not interact with the MCP server directly after initial setup.

### Operator
The person who deploys and maintains the MCP server infrastructure. They use Terraform via Makefile targets to provision and update the deployment.

## 4. Usage Scenarios

### SC-001: Search Files
**Actor:** AI Agent
**Preconditions:** Agent has a valid Bearer token; MCP session established
**Flow:**
1. Agent sends `tools/call` with tool `drive_search`, providing `query` (string), optional `fileTypes` (comma-separated shortcuts or MIME types), optional `maxResults` (integer, default 50)
2. System expands type shortcuts (e.g., "image" -> list of image MIME types) if provided
3. System queries Google Drive Files API with the constructed query
4. System returns list of matching files, each with: `id`, `name`, `mimeType`, `modifiedTime`, `size`
**Postconditions:** Agent receives search results (may be empty)
**Exceptions:**
- EXC-001a: Google API returns 429 (rate limit) -> System returns MCP error with message "Rate limit exceeded, retry after a delay"
- EXC-001b: Google API returns 5xx -> System returns MCP error with message "Google Drive API unavailable"
- EXC-001c: Invalid Bearer token -> System returns HTTP 401 with `WWW-Authenticate` header before MCP layer is reached

### SC-002: List Folder Contents
**Actor:** AI Agent
**Preconditions:** Agent has a valid Bearer token; MCP session established
**Flow:**
1. Agent sends `tools/call` with tool `drive_folder_list`, providing `folderId` (string)
2. System queries Google Drive Files API for items where `folderId` is a parent
3. System sorts results: folders first, then files, alphabetically within each group
4. System returns list of items, each with: `id`, `name`, `mimeType`, `modifiedTime`, `size`
**Postconditions:** Agent receives folder contents (may be empty)
**Exceptions:**
- EXC-002a: `folderId` does not exist -> System returns MCP error "Folder not found: {folderId}"
- EXC-002b: `folderId` refers to a file, not a folder -> System returns MCP error "Not a folder: {folderId}"
- EXC-002c: User lacks permission to access folder -> System returns MCP error "Permission denied"

### SC-003: Get File Info
**Actor:** AI Agent
**Preconditions:** Agent has a valid Bearer token; MCP session established
**Flow:**
1. Agent sends `tools/call` with tool `drive_file_info`, providing `fileId` (string)
2. System queries Google Drive Files API for full metadata (name, ID, MIME type, size, created time, modified time, web view link, owners)
3. System traverses parent chain to reconstruct full path from root
4. System returns complete file information including path
**Postconditions:** Agent receives file metadata with full path
**Exceptions:**
- EXC-003a: `fileId` does not exist -> System returns MCP error "File not found: {fileId}"
- EXC-003b: User lacks permission -> System returns MCP error "Permission denied"

### SC-004: Get Download URL
**Actor:** AI Agent
**Preconditions:** Agent has a valid Bearer token; MCP session established; file is not a Google Workspace file
**Flow:**
1. Agent sends `tools/call` with tool `drive_download_url`, providing `fileId` (string)
2. System retrieves file metadata to verify it exists and is not a Google Workspace file
3. System constructs an authenticated download URL embedding the user's access token as a query parameter
4. System returns the URL with metadata: `downloadUrl`, `fileName`, `mimeType`, `size`, `expiresIn` (seconds, ~3600)
**Postconditions:** Agent receives a time-limited download URL
**Exceptions:**
- EXC-004a: File is a Google Workspace file -> System returns MCP error "Cannot download Google Workspace files directly. Use drive_export_url instead."
- EXC-004b: `fileId` does not exist -> System returns MCP error "File not found: {fileId}"
- EXC-004c: User lacks permission -> System returns MCP error "Permission denied"

### SC-005: Get Export URL
**Actor:** AI Agent
**Preconditions:** Agent has a valid Bearer token; MCP session established; file is a Google Workspace file
**Flow:**
1. Agent sends `tools/call` with tool `drive_export_url`, providing `fileId` (string) and `format` (string: pdf, docx, xlsx, pptx, csv, txt, html)
2. System retrieves file metadata to verify it is a Google Workspace file
3. System validates the requested format is supported for the file's MIME type
4. System constructs an authenticated export URL with the target MIME type and access token
5. System returns: `exportUrl`, `fileName` (with adjusted extension), `exportMimeType`, `expiresIn`
**Postconditions:** Agent receives a time-limited export URL
**Exceptions:**
- EXC-005a: File is not a Google Workspace file -> System returns MCP error "Not a Google Workspace file. Use drive_download_url instead."
- EXC-005b: Unsupported format for file type -> System returns MCP error "Format '{format}' not supported for {mimeType}. Supported: {list}"
- EXC-005c: `fileId` does not exist -> System returns MCP error "File not found: {fileId}"

### SC-006: Create Upload Session
**Actor:** AI Agent
**Preconditions:** Agent has a valid Bearer token; MCP session established; target folder exists
**Flow:**
1. Agent sends `tools/call` with tool `drive_create_upload_url`, providing `fileName` (string), `folderId` (string), optional `mimeType` (string override)
2. System auto-detects MIME type from filename extension if not provided
3. System checks if a file with the same name exists in the target folder
4. If file exists: system creates a resumable upload session to update the existing file (new version)
5. If file does not exist: system creates a resumable upload session for a new file
6. System returns: `uploadUrl` (resumable upload URI), `fileId` (if updating existing), `isUpdate` (boolean), `detectedMimeType`
**Postconditions:** Agent receives a resumable upload URL; caller handles the actual data upload via PUT
**Exceptions:**
- EXC-006a: `folderId` does not exist -> System returns MCP error "Folder not found: {folderId}"
- EXC-006b: `folderId` refers to a file -> System returns MCP error "Not a folder: {folderId}"
- EXC-006c: MIME type cannot be detected and not provided -> System defaults to `application/octet-stream`

### SC-007: Delete File (Trash)
**Actor:** AI Agent
**Preconditions:** Agent has a valid Bearer token; MCP session established
**Flow:**
1. Agent sends `tools/call` with tool `drive_delete`, providing `fileId` (string)
2. System moves the file to Google Drive trash via Files.Update with `trashed: true`
3. System returns: `fileId`, `fileName`, `message` ("File moved to trash")
**Postconditions:** File is in trash (recoverable)
**Exceptions:**
- EXC-007a: `fileId` does not exist -> System returns MCP error "File not found: {fileId}"
- EXC-007b: File is already trashed -> System returns MCP error "File is already in trash"
- EXC-007c: User lacks permission to trash -> System returns MCP error "Permission denied"

### SC-008: Rename File
**Actor:** AI Agent
**Preconditions:** Agent has a valid Bearer token; MCP session established
**Flow:**
1. Agent sends `tools/call` with tool `drive_rename`, providing `fileId` (string) and `newName` (string)
2. System updates file name via Files.Update
3. System returns updated metadata: `id`, `name`, `mimeType`, `modifiedTime`, `webViewLink`
**Postconditions:** File is renamed
**Exceptions:**
- EXC-008a: `fileId` does not exist -> System returns MCP error "File not found: {fileId}"
- EXC-008b: Empty `newName` -> System returns MCP error "newName is required"
- EXC-008c: User lacks edit permission -> System returns MCP error "Permission denied"

### SC-009: Move File
**Actor:** AI Agent
**Preconditions:** Agent has a valid Bearer token; MCP session established
**Flow:**
1. Agent sends `tools/call` with tool `drive_move`, providing `fileId` (string) and `targetFolderId` (string)
2. System retrieves current parents of the file
3. System updates file via Files.Update, adding `targetFolderId` and removing old parents
4. System returns updated metadata: `id`, `name`, `mimeType`, `modifiedTime`
**Postconditions:** File is in the new folder; removed from old folder
**Exceptions:**
- EXC-009a: `fileId` does not exist -> System returns MCP error "File not found: {fileId}"
- EXC-009b: `targetFolderId` does not exist -> System returns MCP error "Target folder not found: {targetFolderId}"
- EXC-009c: `targetFolderId` is not a folder -> System returns MCP error "Not a folder: {targetFolderId}"

### SC-010: Copy File
**Actor:** AI Agent
**Preconditions:** Agent has a valid Bearer token; MCP session established
**Flow:**
1. Agent sends `tools/call` with tool `drive_copy`, providing `fileId` (string), `targetFolderId` (string, required), optional `newName` (string)
2. System creates a copy via Files.Copy, setting parent to `targetFolderId` and name to `newName` (or original name if not provided)
3. System returns new file metadata: `id`, `name`, `mimeType`, `modifiedTime`, `webViewLink`
**Postconditions:** A new copy of the file exists in the target folder
**Exceptions:**
- EXC-010a: `fileId` does not exist -> System returns MCP error "File not found: {fileId}"
- EXC-010b: `targetFolderId` does not exist -> System returns MCP error "Target folder not found: {targetFolderId}"
- EXC-010c: Missing `targetFolderId` -> System returns MCP error "targetFolderId is required"

### SC-011: Create Folder
**Actor:** AI Agent
**Preconditions:** Agent has a valid Bearer token; MCP session established
**Flow:**
1. Agent sends `tools/call` with tool `drive_folder_create`, providing `parentFolderId` (string) and `name` (string)
2. System creates a new folder with MIME type `application/vnd.google-apps.folder` under the parent
3. System returns: `id`, `name`, `mimeType`, `webViewLink`
**Postconditions:** New folder exists under the parent
**Exceptions:**
- EXC-011a: `parentFolderId` does not exist -> System returns MCP error "Parent folder not found: {parentFolderId}"
- EXC-011b: `parentFolderId` is not a folder -> System returns MCP error "Not a folder: {parentFolderId}"
- EXC-011c: Empty `name` -> System returns MCP error "name is required"

### SC-012: List Permissions
**Actor:** AI Agent
**Preconditions:** Agent has a valid Bearer token; MCP session established
**Flow:**
1. Agent sends `tools/call` with tool `drive_permissions_list`, providing `fileId` (string)
2. System queries Permissions.List for the file
3. System returns list of permissions, each with: `id`, `type` (user/group/domain/anyone), `role`, `emailAddress`, `displayName`, `domain`
**Postconditions:** Agent receives permission list (may be empty beyond owner)
**Exceptions:**
- EXC-012a: `fileId` does not exist -> System returns MCP error "File not found: {fileId}"
- EXC-012b: User lacks permission to view permissions -> System returns MCP error "Permission denied"

### SC-013: Update Permissions
**Actor:** AI Agent
**Preconditions:** Agent has a valid Bearer token; MCP session established
**Flow:**
1. Agent sends `tools/call` with tool `drive_permissions_update`, providing `fileId` (string) and `action` ("add" or "remove")
2. **For add**: also provides `type` ("user" or "anyone"), `role` ("reader", "writer", "commenter"), optional `email` (required if type=user), optional `notify` (boolean, default true), optional `message` (string)
3. System creates the permission via Permissions.Create (for user: emailAddress + role + sendNotificationEmail; for anyone: type=anyone + role)
4. **For remove**: also provides `permissionId` (string)
5. System deletes the permission via Permissions.Delete
6. System returns updated permissions list (same format as SC-012)
**Postconditions:** Permission is added or removed
**Exceptions:**
- EXC-013a: `fileId` does not exist -> System returns MCP error "File not found: {fileId}"
- EXC-013b: Invalid `action` -> System returns MCP error "action must be 'add' or 'remove'"
- EXC-013c: Missing `email` when type=user -> System returns MCP error "email is required when type is 'user'"
- EXC-013d: Missing `permissionId` for remove -> System returns MCP error "permissionId is required for remove action"
- EXC-013e: `permissionId` does not exist -> System returns MCP error "Permission not found: {permissionId}"
- EXC-013f: User lacks permission to modify sharing -> System returns MCP error "Permission denied"

### SC-014: List Recent Changes
**Actor:** AI Agent
**Preconditions:** Agent has a valid Bearer token; MCP session established
**Flow:**
1. Agent sends `tools/call` with tool `drive_activity_changes`, optional `maxResults` (integer, default 50)
2. System queries Changes API with start page token
3. System returns list of changes, each with: `fileId`, `fileName`, `changeType` (Added/Modified/Removed), `changeTime`, `modifiedBy`
**Postconditions:** Agent receives change list
**Exceptions:**
- EXC-014a: Google API rate limit -> System returns MCP error with retry suggestion

### SC-015: List Deleted Files
**Actor:** AI Agent
**Preconditions:** Agent has a valid Bearer token; MCP session established
**Flow:**
1. Agent sends `tools/call` with tool `drive_activity_deleted`, optional `daysBack` (integer, default 7), optional `maxResults` (integer, default 100)
2. System queries Files API for trashed files within the time window
3. System returns list of trashed files, each with: `id`, `name`, `trashedTime`, `size`, `trashedBy`
**Postconditions:** Agent receives deleted files list
**Exceptions:**
- EXC-015a: No trashed files in time window -> System returns empty list (not an error)

### SC-016: Query Activity History
**Actor:** AI Agent
**Preconditions:** Agent has a valid Bearer token; MCP session established
**Flow:**
1. Agent sends `tools/call` with tool `drive_activity_history`, optional `daysBack` (integer, default 7), optional `maxResults` (integer, default 100, hard cap 200)
2. System enforces hard cap: if `maxResults` > 200, silently clamp to 200
3. System queries Drive Activity API v2 with pagination and rate limit handling
4. System returns list of activities, each with: `timestamp`, `actionType`, `actionDetail`, `actors`, `targets`, `targetTitles`
**Postconditions:** Agent receives activity history (capped at 200)
**Exceptions:**
- EXC-016a: Rate limit during pagination -> System retries with exponential backoff (up to 3 retries)
- EXC-016b: Exceeds Cloud Run timeout despite cap -> System returns partial results collected so far with a warning message

### SC-017: List File Revisions
**Actor:** AI Agent
**Preconditions:** Agent has a valid Bearer token; MCP session established
**Flow:**
1. Agent sends `tools/call` with tool `drive_file_revisions`, providing `fileId` (string)
2. System queries Revisions API for the file
3. System returns list of revisions, each with: `id`, `modifiedTime`, `size`, `modifiedBy`, `keepForever`
**Postconditions:** Agent receives revision list (newest first)
**Exceptions:**
- EXC-017a: `fileId` does not exist -> System returns MCP error "File not found: {fileId}"
- EXC-017b: File does not support revisions (e.g., Google Workspace files with limited history) -> System returns empty list with info message

### SC-018: OAuth2 Authentication
**Actor:** Human User (browser), MCP Client (programmatic)
**Preconditions:** MCP server is running; OAuth credentials are loaded from Secret Manager or local file
**Flow:**
1. MCP client discovers auth server via `GET /.well-known/oauth-protected-resource` (RFC 9728)
2. MCP client fetches auth server metadata from `GET /.well-known/oauth-authorization-server` (RFC 8414)
3. MCP client registers via `POST /oauth/register` (RFC 7591 Dynamic Client Registration)
4. MCP client redirects user to `GET /oauth/authorize` with `client_id`, `redirect_uri`, `response_type=code`, `state`, `code_challenge`, `code_challenge_method=S256`
5. Server stores authorization state and redirects user to Google OAuth consent page
6. User grants consent; Google redirects to `GET /oauth/callback` with authorization code
7. Server exchanges code with Google for tokens (access + refresh)
8. Server generates its own authorization code and redirects to client's `redirect_uri`
9. MCP client exchanges code at `POST /oauth/token` with `code_verifier` for PKCE validation
10. Server validates PKCE, returns Google access token and refresh token to client
11. MCP client uses access token as Bearer token for subsequent MCP requests
**Postconditions:** MCP client has valid access token and refresh token
**Exceptions:**
- EXC-018a: User denies consent -> Google redirects with `error=access_denied`; server returns error to client
- EXC-018b: State expired (>10 min) -> Server returns "Invalid or expired state"
- EXC-018c: No refresh token returned -> Server returns error "No refresh token received"
- EXC-018d: PKCE verification fails -> Server returns "Invalid code_verifier"
- EXC-018e: Secret Manager unavailable -> Server logs error, falls back to local credential file
- EXC-018f: Authorization code expired -> Server returns "Invalid or expired authorization code"
**Cross-scenario notes:** Every tool call in SC-001 through SC-017 depends on a valid token from this flow. Token refresh is handled transparently by the OAuth2 library.

### SC-019: Server Deployment
**Actor:** Operator
**Preconditions:** GCP project exists; `gcloud` CLI authenticated; Docker daemon running; Terraform installed
**Flow:**
1. Operator edits `config.yaml` with desired GCP project ID, region, and resource names
2. Operator runs `make init-plan` to preview initialization resources (state backend, service accounts, API enablement)
3. Operator runs `make init-deploy` to create initialization resources
4. Operator manually uploads OAuth credentials to Secret Manager
5. Operator runs `make plan` to preview main infrastructure (Cloud Run, Firestore, DNS, Docker build)
6. Operator runs `make deploy` to build Docker image, push to Artifact Registry, and deploy Cloud Run service with domain mapping
7. System provisions Cloud DNS zone and records for `drive.mcp.scm-platform.org`
**Postconditions:** MCP server is accessible at `https://drive.mcp.scm-platform.org`
**Exceptions:**
- EXC-019a: Docker daemon not running -> Terraform fails at Docker build step
- EXC-019b: `gcloud` not authenticated -> Terraform fails at provider initialization
- EXC-019c: DNS zone already exists in another project -> Terraform fails; operator must import or clean up
- EXC-019d: Insufficient GCP permissions -> Terraform fails with permission error

### SC-020: Health Check
**Actor:** Monitoring system
**Preconditions:** MCP server is running
**Flow:**
1. Monitoring sends `GET /health`
2. Server responds with HTTP 200 and body "OK"
**Postconditions:** Monitoring confirms server is alive
**Exceptions:**
- EXC-020a: Server is down -> No response / connection refused
- EXC-020b: Cloud Run cold start -> Response delayed but succeeds

## 5. Functional Requirements

### FR-001: MCP Server Subcommand
- **Description:** The `gdrive` binary must support a `mcp` subcommand that starts an MCP HTTP Streamable server
- **Inputs:** `--port` (int, default 8080), `--host` (string, default "0.0.0.0"), `--base-url` (string, required in production), `--secret-name` (string), `--secret-project` (string), `--credential-file` (string, fallback)
- **Outputs:** HTTP server listening on specified host:port
- **Business Rules:** Server must use `modelcontextprotocol/go-sdk` with Streamable HTTP transport; `Stateless: false` for session tracking; graceful shutdown on SIGINT/SIGTERM
- **Priority:** Must-have

### FR-002: OAuth2 Authorization Server
- **Description:** MCP server must implement an OAuth 2.1 authorization server acting as a proxy to Google OAuth
- **Inputs:** HTTP requests to well-known, register, authorize, callback, and token endpoints
- **Outputs:** JSON responses per RFC specifications; redirects during OAuth flow
- **Business Rules:**
  - Must implement RFC 9728 (Protected Resource Metadata) at `/.well-known/oauth-protected-resource`
  - Must implement RFC 8414 (Authorization Server Metadata) at `/.well-known/oauth-authorization-server`
  - Must implement RFC 7591 (Dynamic Client Registration) at `POST /oauth/register`
  - Must support PKCE S256 (RFC 7636) for authorization code exchange
  - Must support `authorization_code` and `refresh_token` grant types
  - OAuth credentials loaded from Secret Manager (priority) or local file (fallback)
  - In-memory stores for registered clients, authorization states, and authorization codes with TTL cleanup
  - Auto-register unknown clients on `/oauth/authorize` (same as google-contacts pattern)
- **Priority:** Must-have

### FR-003: Authentication Middleware
- **Description:** MCP endpoint (`/`) must be protected by Bearer token authentication
- **Inputs:** `Authorization: Bearer <token>` header on MCP requests
- **Outputs:** Authenticated request with OAuth config and token injected into context; or HTTP 401 with `WWW-Authenticate` header
- **Business Rules:**
  - Missing token returns 401 with `WWW-Authenticate: Bearer resource_metadata="{baseURL}/.well-known/oauth-protected-resource"`
  - Invalid token returns 401 with `WWW-Authenticate: Bearer error="invalid_token", resource_metadata="..."`
  - Valid token: inject `oauth2.Config` and `oauth2.Token` into request context
  - Health and OAuth endpoints are NOT protected
- **Priority:** Must-have

### FR-004: Context-Based Token Injection
- **Description:** The auth package must support injecting OAuth tokens via context for MCP server use, while maintaining CLI file-based token flow
- **Inputs:** Context with OAuth config and access token (MCP mode) or config struct with file paths (CLI mode)
- **Outputs:** Authenticated `*http.Client` suitable for Google API calls
- **Business Rules:**
  - Add `WithOAuthConfig(ctx, config)`, `GetOAuthConfigFromContext(ctx)` functions
  - Add `WithAccessToken(ctx, token)`, `GetAccessTokenFromContext(ctx)` functions
  - Drive service creation must check context first, then fall back to file-based auth
  - No changes to existing CLI authentication flow
- **Priority:** Must-have

### FR-005: Drive Search Tool
- **Description:** MCP tool `drive_search` that searches Google Drive files by query
- **Inputs:** `query` (string, required), `fileTypes` (string, optional, comma-separated), `maxResults` (integer, optional, default 50)
- **Outputs:** Array of file objects: `{id, name, mimeType, modifiedTime, size}`
- **Business Rules:** Type shortcuts (image, audio, video, prez, doc, spreadsheet, txt, pdf, folder) must be expanded to corresponding MIME types, same as CLI; explicit MIME types also accepted
- **Priority:** Must-have

### FR-006: Folder List Tool
- **Description:** MCP tool `drive_folder_list` that lists contents of a folder
- **Inputs:** `folderId` (string, required)
- **Outputs:** Array of items: `{id, name, mimeType, modifiedTime, size}`, sorted folders-first then alphabetical
- **Business Rules:** Must validate `folderId` exists and is a folder
- **Priority:** Must-have

### FR-007: File Info Tool
- **Description:** MCP tool `drive_file_info` that returns detailed file metadata
- **Inputs:** `fileId` (string, required)
- **Outputs:** `{id, name, mimeType, size, createdTime, modifiedTime, webViewLink, owners[], path[]}`
- **Business Rules:** Path reconstruction must traverse parent chain to root; always included (no opt-out)
- **Priority:** Must-have

### FR-008: Download URL Tool
- **Description:** MCP tool `drive_download_url` that returns an authenticated download URL
- **Inputs:** `fileId` (string, required)
- **Outputs:** `{downloadUrl, fileName, mimeType, size, expiresIn}`
- **Business Rules:** Must reject Google Workspace files (direct the agent to `drive_export_url`); URL embeds user's access token as query parameter; `expiresIn` reflects token expiry (~3600s)
- **Priority:** Must-have

### FR-009: Export URL Tool
- **Description:** MCP tool `drive_export_url` that returns an authenticated export URL for Google Workspace files
- **Inputs:** `fileId` (string, required), `format` (string, required: pdf/docx/xlsx/pptx/csv/txt/html)
- **Outputs:** `{exportUrl, fileName, exportMimeType, expiresIn}`
- **Business Rules:** Must reject non-Workspace files; must validate format is supported for the specific Workspace file type; filename extension adjusted to match export format; supported format mappings match CLI's `ExportFormats` map
- **Priority:** Must-have

### FR-010: Upload URL Tool
- **Description:** MCP tool `drive_create_upload_url` that creates a resumable upload session
- **Inputs:** `fileName` (string, required), `folderId` (string, required), `mimeType` (string, optional)
- **Outputs:** `{uploadUrl, fileId, isUpdate, detectedMimeType}`
- **Business Rules:** Auto-detect MIME type from extension if not provided (fallback: `application/octet-stream`); if file with same name exists in folder, create update session (new version); return `isUpdate: true` and existing `fileId` in that case
- **Priority:** Must-have

### FR-011: Delete Tool
- **Description:** MCP tool `drive_delete` that moves a file to trash
- **Inputs:** `fileId` (string, required)
- **Outputs:** `{fileId, fileName, message}`
- **Business Rules:** Soft delete only (set `trashed: true`); no permanent deletion; must detect already-trashed files
- **Priority:** Must-have

### FR-012: Rename Tool
- **Description:** MCP tool `drive_rename` that renames a file
- **Inputs:** `fileId` (string, required), `newName` (string, required)
- **Outputs:** `{id, name, mimeType, modifiedTime, webViewLink}`
- **Business Rules:** Must validate non-empty `newName`
- **Priority:** Must-have

### FR-013: Move Tool
- **Description:** MCP tool `drive_move` that moves a file to a different folder
- **Inputs:** `fileId` (string, required), `targetFolderId` (string, required)
- **Outputs:** `{id, name, mimeType, modifiedTime}`
- **Business Rules:** Must validate `targetFolderId` exists and is a folder; removes from all current parents and adds to target
- **Priority:** Must-have

### FR-014: Copy Tool
- **Description:** MCP tool `drive_copy` that duplicates a file
- **Inputs:** `fileId` (string, required), `targetFolderId` (string, required), `newName` (string, optional)
- **Outputs:** `{id, name, mimeType, modifiedTime, webViewLink}`
- **Business Rules:** `targetFolderId` is required (no implicit same-folder copy); if `newName` not provided, uses original name
- **Priority:** Must-have

### FR-015: Folder Create Tool
- **Description:** MCP tool `drive_folder_create` that creates a single folder
- **Inputs:** `parentFolderId` (string, required), `name` (string, required)
- **Outputs:** `{id, name, mimeType, webViewLink}`
- **Business Rules:** Creates one folder; agent calls multiple times for nested paths; must validate parent exists and is a folder
- **Priority:** Must-have

### FR-016: Permissions List Tool
- **Description:** MCP tool `drive_permissions_list` that lists all permissions on a file
- **Inputs:** `fileId` (string, required)
- **Outputs:** Array of permissions: `{id, type, role, emailAddress, displayName, domain}`
- **Business Rules:** Includes all permission types (user, group, domain, anyone); `SupportsAllDrives(true)` for shared drive support
- **Priority:** Must-have

### FR-017: Permissions Update Tool
- **Description:** MCP tool `drive_permissions_update` that adds or removes permissions
- **Inputs:** `fileId` (string, required), `action` (string, required: "add"/"remove"); for add: `type` ("user"/"anyone"), `role` ("reader"/"writer"/"commenter"), `email` (string, required if type=user), `notify` (bool, default true), `message` (string, optional); for remove: `permissionId` (string, required)
- **Outputs:** Updated permissions list (same format as FR-016)
- **Business Rules:** Validate all required fields per action type; notification parameters only used for add+user; `SupportsAllDrives(true)`
- **Priority:** Must-have

### FR-018: Activity Changes Tool
- **Description:** MCP tool `drive_activity_changes` that lists recent Drive changes
- **Inputs:** `maxResults` (integer, optional, default 50)
- **Outputs:** Array of changes: `{fileId, fileName, changeType, changeTime, modifiedBy}`
- **Business Rules:** Uses Changes API with start page token; change types: Added, Modified, Removed
- **Priority:** Must-have

### FR-019: Activity Deleted Tool
- **Description:** MCP tool `drive_activity_deleted` that lists trashed files
- **Inputs:** `daysBack` (integer, optional, default 7), `maxResults` (integer, optional, default 100)
- **Outputs:** Array of trashed files: `{id, name, trashedTime, size, trashedBy}`
- **Business Rules:** Ordered by trashedTime descending (newest first)
- **Priority:** Must-have

### FR-020: Activity History Tool
- **Description:** MCP tool `drive_activity_history` that queries Drive Activity API
- **Inputs:** `daysBack` (integer, optional, default 7), `maxResults` (integer, optional, default 100, hard cap 200)
- **Outputs:** Array of activities: `{timestamp, actionType, actionDetail, actors[], targets[], targetTitles[]}`
- **Business Rules:** Hard cap of 200 results (silently clamp); exponential backoff on rate limits (up to 3 retries); must not use `fmt.Fprintf(os.Stderr, ...)` for rate limit messages (use `slog` instead)
- **Priority:** Must-have

### FR-021: File Revisions Tool
- **Description:** MCP tool `drive_file_revisions` that lists revision history for a file
- **Inputs:** `fileId` (string, required)
- **Outputs:** Array of revisions: `{id, modifiedTime, size, modifiedBy, keepForever}`, newest first
- **Business Rules:** Must validate `fileId` exists
- **Priority:** Must-have

### FR-022: Health Check Endpoint
- **Description:** `GET /health` endpoint that returns server status
- **Inputs:** HTTP GET request (no authentication)
- **Outputs:** HTTP 200 with body "OK"
- **Business Rules:** Must not require authentication; used by Cloud Run health checks and external monitoring
- **Priority:** Must-have

### FR-023: Structured Logging
- **Description:** All server logging must use Go's `slog` package
- **Inputs:** Log events from all server components
- **Outputs:** Structured log entries to stdout
- **Business Rules:** JSON format when `ENVIRONMENT=prd`; text format otherwise; log level configurable via `LOG_LEVEL` env var (default: info); all tool calls must log tool name, duration, and error status
- **Priority:** Must-have

### FR-024: Terraform Infrastructure
- **Description:** Complete Terraform configuration for deploying the MCP server
- **Inputs:** `config.yaml` with GCP project ID, region, resource names
- **Outputs:** Provisioned Cloud Run service, Firestore database, Secret Manager secret, Artifact Registry, Cloud DNS zone and records, domain mapping
- **Business Rules:**
  - `config.yaml` is the single source of truth
  - Three-phase deployment: `init/` (state backend, service accounts, API enablement), `iac/` (application infrastructure), Docker build
  - Makefile with 6 targets: `init-plan`, `init-deploy`, `init-destroy`, `plan`, `deploy`, `undeploy`
  - Cloud DNS must manage `drive.mcp.scm-platform.org` with correct records for Cloud Run domain mapping
  - Changing `project_id` in `config.yaml` and redeploying must work (portable infrastructure)
  - GCP APIs to enable: `run.googleapis.com`, `firestore.googleapis.com`, `secretmanager.googleapis.com`, `cloudbuild.googleapis.com`, `artifactregistry.googleapis.com`, `cloudresourcemanager.googleapis.com`, `iam.googleapis.com`, `dns.googleapis.com`
- **Priority:** Must-have

### FR-025: Ping Tool
- **Description:** MCP tool `ping` for connectivity testing
- **Inputs:** None
- **Outputs:** `{message: "pong", time: "<RFC3339 timestamp>"}`
- **Business Rules:** Must be the simplest tool; useful for verifying MCP session is working
- **Priority:** Must-have

## 6. Non-Functional Requirements

### 6.1 Performance
- Health endpoint must respond within 100ms
- MCP `initialize` + `tools/list` must complete within 2 seconds
- Individual tool calls (excluding activity history) must complete within 30 seconds
- Activity history with 200 cap must complete within 300 seconds (Cloud Run timeout)
- Cloud Run cold start must not exceed 10 seconds

### 6.2 Security
- All MCP tool endpoints require OAuth2 Bearer token authentication
- OAuth credentials stored in GCP Secret Manager (not in code or environment variables)
- PKCE S256 required for authorization code exchange
- In-memory OAuth state entries expire after 10 minutes
- Authorization codes are single-use and expire after 10 minutes
- No shared secrets or code with google-contacts project
- HTTPS enforced by Cloud Run (TLS termination at load balancer)
- Access tokens embedded in signed URLs expire in ~1 hour

### 6.3 Usability
- MCP tool descriptions must be clear and complete for AI agent consumption
- Tool input schemas must use `jsonschema` tags with descriptive text
- Error messages must be actionable (e.g., "Use drive_export_url instead" not just "unsupported")
- Tool output schemas must always initialize slices to empty arrays (never `null` in JSON)

### 6.4 Reliability
- Graceful shutdown on SIGINT/SIGTERM with 10-second timeout
- Exponential backoff on Google API rate limits (up to 3 retries)
- OAuth credential loading falls back from Secret Manager to local file
- Cloud Run min instances: 0 (scale to zero when idle)
- Cloud Run max instances: 3 (prevent runaway scaling)

### 6.5 Observability
- Structured logging via `slog` (JSON in production, text locally)
- All tool calls logged with: tool name, duration, success/error
- OAuth flow events logged: authorization requests, callbacks, token exchanges
- Log level configurable via `LOG_LEVEL` environment variable
- Cloud Run logs automatically forwarded to Cloud Logging

### 6.6 Deployment
- Dockerfile: multi-stage build (Go builder -> scratch/distroless)
- Cloud Run in `europe-west1` (configurable via `config.yaml`)
- Artifact Registry for Docker images
- Custom domain: `drive.mcp.scm-platform.org` via Cloud Run domain mapping + Cloud DNS
- Three-phase Terraform: `init/` -> `iac/` -> Docker build
- Makefile with 6 targets for infrastructure management
- `config.yaml` as single source of truth for all Terraform configuration

### 6.7 Scalability
- Stateless MCP sessions (in-memory OAuth state is ephemeral and recreatable)
- Cloud Run auto-scaling 0-3 instances
- Firestore for token persistence (no local state)
- Each request creates its own Google API client from the injected token (no shared state)

## 7. Data Model

### 7.1 Firestore Collections

**Collection: `api_keys`** (same pattern as google-contacts)

| Field | Type | Description |
|-------|------|-------------|
| Document ID | string | API key (UUID v4) |
| `refresh_token` | string | Google OAuth refresh token (required) |
| `access_token` | string | Cached Google access token (optional) |
| `token_expiry` | string | Access token expiry timestamp (optional) |
| `user_email` | string | User's Google email (optional) |
| `created_at` | string | Key creation timestamp (optional) |
| `last_used` | string | Last usage timestamp (optional) |
| `description` | string | Human-readable description (optional) |

### 7.2 In-Memory Stores (OAuth2 Server)

**Registered Clients**: Map of client_id -> {clientId, clientSecret, redirectURIs[], createdAt}

**Authorization States**: Map of state -> {clientId, redirectURI, codeChallenge, codeMethod, createdAt} (TTL: 10 min)

**Authorization Codes**: Map of code -> {code, clientId, redirectURI, codeChallenge, codeMethod, googleToken, createdAt} (TTL: 10 min)

### 7.3 Secret Manager

| Secret | Description |
|--------|-------------|
| `scm-pwd-gdrive-oauth-creds` | Google OAuth client credentials JSON (client_id, client_secret) |

## 8. Documentation Requirements

All documentation listed below must be created and maintained as part of this project.

### 8.1 README.md
- Project description (CLI + MCP server)
- Prerequisites and installation instructions
- CLI usage examples (existing)
- MCP server usage: `gdrive mcp --port 8080`
- Infrastructure deployment guide (Terraform)
- Configuration reference (`config.yaml`)
- MCP tool reference (all 20 tools with examples)

### 8.2 CLAUDE.md & .agent_docs/
- `CLAUDE.md`: Update with MCP server architecture, new packages, new commands
- `.agent_docs/mcp-server.md`: MCP server documentation (tools, auth, endpoints)
- `.agent_docs/terraform.md`: Infrastructure documentation
- `.agent_docs/authentication.md`: OAuth2 flow documentation (CLI + MCP modes)
- Must be kept in sync with code changes

### 8.3 docs/
- Not required for v1 (README.md and .agent_docs/ are sufficient)

## 9. Traceability Matrix

| Scenario | Functional Req | E2E Tests (Happy) | E2E Tests (Failure) | E2E Tests (Edge) |
|----------|---------------|-------------------|---------------------|-------------------|
| SC-001 | FR-005 | E2E-001, E2E-011, E2E-012 | E2E-021 | E2E-035, E2E-038, E2E-041 |
| SC-002 | FR-006 | E2E-002, E2E-008 | E2E-025 | E2E-036 |
| SC-003 | FR-007 | E2E-003 | E2E-021, E2E-022 | E2E-037 |
| SC-004 | FR-008 | E2E-004 | E2E-021, E2E-022, E2E-023 | -- |
| SC-005 | FR-009 | E2E-005, E2E-013 | E2E-023, E2E-024 | -- |
| SC-006 | FR-010 | E2E-006, E2E-014, E2E-015, E2E-016 | E2E-025 | E2E-040 |
| SC-007 | FR-011 | E2E-007 | E2E-021, E2E-026 | -- |
| SC-008 | FR-012 | E2E-007 | E2E-021 | -- |
| SC-009 | FR-013 | E2E-007 | E2E-021, E2E-027 | -- |
| SC-010 | FR-014 | E2E-007 | E2E-021, E2E-028 | -- |
| SC-011 | FR-015 | E2E-008 | E2E-025 | E2E-039 |
| SC-012 | FR-016 | E2E-009 | E2E-021 | -- |
| SC-013 | FR-017 | E2E-009, E2E-017, E2E-018 | E2E-029 | -- |
| SC-014 | FR-018 | E2E-010 | E2E-021 | -- |
| SC-015 | FR-019 | E2E-010, E2E-019 | -- | -- |
| SC-016 | FR-020 | E2E-010, E2E-020 | -- | E2E-042 |
| SC-017 | FR-021 | E2E-010 | E2E-021 | E2E-043 |
| SC-018 | FR-002, FR-003, FR-004 | E2E-045, E2E-046, E2E-047, E2E-048 | E2E-030, E2E-031, E2E-032, E2E-033, E2E-034, E2E-049 | E2E-044 |
| SC-019 | FR-024 | -- (infra, not E2E testable) | -- | -- |
| SC-020 | FR-022 | E2E-050 | -- | -- |
| -- | FR-001 | E2E-051 | -- | -- |
| -- | FR-023 | (verified via log inspection in other tests) | -- | -- |
| -- | FR-025 | E2E-051 | -- | -- |

## 10. End-to-End Test Suite

All tests must be implemented in the `tests/` directory using Go build tags `//go:build e2e`. Tests use a real HTTP server on a dedicated port.

### 10.1 Test Summary

| Test ID | Category | Scenario | FR refs | Priority |
|---------|----------|----------|---------|----------|
| E2E-001 | Core Journey | SC-001 | FR-005 | Critical |
| E2E-002 | Core Journey | SC-002 | FR-006 | Critical |
| E2E-003 | Core Journey | SC-003 | FR-007 | Critical |
| E2E-004 | Core Journey | SC-004 | FR-008 | Critical |
| E2E-005 | Core Journey | SC-005 | FR-009 | Critical |
| E2E-006 | Core Journey | SC-006 | FR-010 | Critical |
| E2E-007 | Core Journey | SC-007-010 | FR-011-014 | Critical |
| E2E-008 | Core Journey | SC-011, SC-002 | FR-015, FR-006 | Critical |
| E2E-009 | Core Journey | SC-012, SC-013 | FR-016, FR-017 | Critical |
| E2E-010 | Core Journey | SC-014-017 | FR-018-021 | Critical |
| E2E-011 | Feature | SC-001 | FR-005 | High |
| E2E-012 | Feature | SC-001 | FR-005 | High |
| E2E-013 | Feature | SC-005 | FR-009 | High |
| E2E-014 | Feature | SC-006 | FR-010 | High |
| E2E-015 | Feature | SC-006 | FR-010 | High |
| E2E-016 | Feature | SC-006 | FR-010 | High |
| E2E-017 | Feature | SC-013 | FR-017 | High |
| E2E-018 | Feature | SC-013 | FR-017 | High |
| E2E-019 | Feature | SC-015 | FR-019 | Medium |
| E2E-020 | Feature | SC-016 | FR-020 | High |
| E2E-021 | Error | SC-003 | FR-007 | Critical |
| E2E-022 | Error | SC-003 | FR-007, FR-008 | High |
| E2E-023 | Error | SC-004, SC-005 | FR-008, FR-009 | High |
| E2E-024 | Error | SC-005 | FR-009 | High |
| E2E-025 | Error | SC-011 | FR-015, FR-006, FR-010 | High |
| E2E-026 | Error | SC-007 | FR-011 | Medium |
| E2E-027 | Error | SC-009 | FR-013 | High |
| E2E-028 | Error | SC-010 | FR-014 | High |
| E2E-029 | Error | SC-013 | FR-017 | Medium |
| E2E-030 | Error | SC-018 | FR-003 | Critical |
| E2E-031 | Error | SC-018 | FR-003 | Critical |
| E2E-032 | Error | SC-018 | FR-003 | Critical |
| E2E-033 | Error | SC-018 | FR-002 | High |
| E2E-034 | Error | SC-018 | FR-002 | High |
| E2E-035 | Edge | SC-001 | FR-005 | Medium |
| E2E-036 | Edge | SC-002 | FR-006 | Medium |
| E2E-037 | Edge | SC-003 | FR-007 | Medium |
| E2E-038 | Edge | SC-001 | FR-005 | Medium |
| E2E-039 | Edge | SC-011 | FR-015 | Medium |
| E2E-040 | Edge | SC-006 | FR-010 | Low |
| E2E-041 | Edge | SC-001 | FR-005 | Medium |
| E2E-042 | Edge | SC-016 | FR-020 | Low |
| E2E-043 | Edge | SC-017 | FR-021 | Low |
| E2E-044 | Edge | SC-018 | FR-003, FR-004 | High |
| E2E-045 | Security | SC-018 | FR-002 | Critical |
| E2E-046 | Security | SC-018 | FR-002 | Critical |
| E2E-047 | Security | SC-018 | FR-002 | High |
| E2E-048 | Security | SC-018 | FR-002 | Critical |
| E2E-049 | Security | SC-018 | FR-002 | Critical |
| E2E-050 | Performance | SC-020 | FR-022 | High |
| E2E-051 | Performance | SC-001 | FR-001, FR-025 | High |
| E2E-052 | Performance | SC-016 | FR-020 | Medium |
| E2E-053 | Cross-Scenario | SC-001,003,004 | FR-005,007,008 | High |
| E2E-054 | Cross-Scenario | SC-011,006,002 | FR-015,010,006 | High |
| E2E-055 | Cross-Scenario | SC-013,012 | FR-017,016 | High |

### 10.2 Test Specifications

#### E2E-001: Search Files Returns Results
- **Category:** Core Journey
- **Scenario:** SC-001 -- Search files
- **Requirements:** FR-005
- **Preconditions:** MCP session established with valid Bearer token; test files exist in Drive
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_search` with `query: "test"`
  - Then response contains an array of file objects
  - And each object has `id`, `name`, `mimeType`, `modifiedTime`, `size` fields
  - And results are non-empty
- **Priority:** Critical

#### E2E-002: List Folder Contents
- **Category:** Core Journey
- **Scenario:** SC-002 -- List folder contents
- **Requirements:** FR-006
- **Preconditions:** MCP session established; test folder with known contents exists
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_folder_list` with a known `folderId`
  - Then response contains items in the folder
  - And folders appear before files in the results
  - And each item has `id`, `name`, `mimeType`, `modifiedTime`, `size` fields
- **Priority:** Critical

#### E2E-003: Get File Info With Full Path
- **Category:** Core Journey
- **Scenario:** SC-003 -- Get file info
- **Requirements:** FR-007
- **Preconditions:** MCP session established; test file exists in a nested folder
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_file_info` with a known `fileId`
  - Then response contains complete metadata: `id`, `name`, `mimeType`, `size`, `createdTime`, `modifiedTime`, `webViewLink`, `owners`, `path`
  - And `path` is a non-empty array representing the folder hierarchy from root
- **Priority:** Critical

#### E2E-004: Get Download URL for Regular File
- **Category:** Core Journey
- **Scenario:** SC-004 -- Get download URL
- **Requirements:** FR-008
- **Preconditions:** MCP session established; test file (non-Workspace) exists
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_download_url` with a known `fileId`
  - Then response contains `downloadUrl`, `fileName`, `mimeType`, `size`, `expiresIn`
  - And `downloadUrl` is a valid HTTPS URL containing an access token
  - And `expiresIn` is approximately 3600
- **Priority:** Critical

#### E2E-005: Export Workspace File as PDF
- **Category:** Core Journey
- **Scenario:** SC-005 -- Get export URL
- **Requirements:** FR-009
- **Preconditions:** MCP session established; Google Docs file exists
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_export_url` with a Google Docs `fileId` and `format: "pdf"`
  - Then response contains `exportUrl`, `fileName`, `exportMimeType`, `expiresIn`
  - And `exportMimeType` is `application/pdf`
  - And `fileName` ends with `.pdf`
- **Priority:** Critical

#### E2E-006: Create Upload Session
- **Category:** Core Journey
- **Scenario:** SC-006 -- Create upload session
- **Requirements:** FR-010
- **Preconditions:** MCP session established; target folder exists
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_create_upload_url` with `fileName: "test.txt"`, `folderId: "<known_id>"`
  - Then response contains `uploadUrl`, `isUpdate`, `detectedMimeType`
  - And `uploadUrl` is a valid HTTPS URL
  - And `detectedMimeType` is `text/plain`
  - And `isUpdate` is false (new file)
- **Priority:** Critical

#### E2E-007: Delete, Rename, Move, Copy File
- **Category:** Core Journey
- **Scenario:** SC-007 to SC-010
- **Requirements:** FR-011, FR-012, FR-013, FR-014
- **Preconditions:** MCP session established; test file exists
- **Steps:**
  - Given an authenticated MCP session and a test file
  - When agent calls `drive_rename` with `fileId` and `newName: "renamed.txt"`
  - Then response contains updated name "renamed.txt"
  - When agent calls `drive_copy` with `fileId`, `targetFolderId`, `newName: "copied.txt"`
  - Then response contains new file with name "copied.txt" and a different `id`
  - When agent calls `drive_move` with the copied file's `id` and a different `targetFolderId`
  - Then response contains the file in the new folder
  - When agent calls `drive_delete` with the copied file's `id`
  - Then response contains `message` including "trash"
- **Priority:** Critical

#### E2E-008: Create Folder and List It
- **Category:** Core Journey
- **Scenario:** SC-011, SC-002
- **Requirements:** FR-015, FR-006
- **Preconditions:** MCP session established; parent folder exists
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_folder_create` with `parentFolderId` and `name: "test-folder"`
  - Then response contains `id`, `name: "test-folder"`, `mimeType: "application/vnd.google-apps.folder"`
  - When agent calls `drive_folder_list` with the new folder's `id`
  - Then response contains an empty items array
- **Priority:** Critical

#### E2E-009: Add and Remove Permissions
- **Category:** Core Journey
- **Scenario:** SC-012, SC-013
- **Requirements:** FR-016, FR-017
- **Preconditions:** MCP session established; test file exists
- **Steps:**
  - Given an authenticated MCP session and a test file
  - When agent calls `drive_permissions_update` with `action: "add"`, `type: "anyone"`, `role: "reader"`
  - Then response contains updated permissions list including an "anyone" entry
  - When agent calls `drive_permissions_list` with the file's `id`
  - Then response contains the "anyone" permission
  - When agent calls `drive_permissions_update` with `action: "remove"`, `permissionId` of the "anyone" entry
  - Then response no longer contains the "anyone" permission
- **Priority:** Critical

#### E2E-010: Query All Activity Tools
- **Category:** Core Journey
- **Scenario:** SC-014 to SC-017
- **Requirements:** FR-018, FR-019, FR-020, FR-021
- **Preconditions:** MCP session established; Drive has recent activity
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_activity_changes` with `maxResults: 10`
  - Then response is an array (possibly empty)
  - When agent calls `drive_activity_deleted` with `daysBack: 7`, `maxResults: 10`
  - Then response is an array (possibly empty)
  - When agent calls `drive_activity_history` with `daysBack: 7`, `maxResults: 10`
  - Then response is an array (possibly empty)
  - When agent calls `drive_file_revisions` with a known `fileId`
  - Then response is an array of revisions
- **Priority:** Critical

#### E2E-011: Search With Type Filter Shortcuts
- **Category:** Feature
- **Scenario:** SC-001
- **Requirements:** FR-005
- **Preconditions:** MCP session established
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_search` with `query: "*"`, `fileTypes: "image"`
  - Then all results have MIME types matching image types (image/jpeg, image/png, etc.)
- **Priority:** High

#### E2E-012: Search With Explicit MIME Type
- **Category:** Feature
- **Scenario:** SC-001
- **Requirements:** FR-005
- **Preconditions:** MCP session established
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_search` with `query: "*"`, `fileTypes: "application/pdf"`
  - Then all results have `mimeType: "application/pdf"`
- **Priority:** High

#### E2E-013: Export Workspace File as DOCX, XLSX, PPTX
- **Category:** Feature
- **Scenario:** SC-005
- **Requirements:** FR-009
- **Preconditions:** MCP session established; Google Docs, Sheets, Slides files exist
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_export_url` with a Docs file and `format: "docx"`
  - Then `exportMimeType` is `application/vnd.openxmlformats-officedocument.wordprocessingml.document`
  - When agent calls `drive_export_url` with a Sheets file and `format: "xlsx"`
  - Then `exportMimeType` is `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet`
  - When agent calls `drive_export_url` with a Slides file and `format: "pptx"`
  - Then `exportMimeType` is `application/vnd.openxmlformats-officedocument.presentationml.presentation`
- **Priority:** High

#### E2E-014: Upload With Auto-Detected MIME Type
- **Category:** Feature
- **Scenario:** SC-006
- **Requirements:** FR-010
- **Preconditions:** MCP session established; target folder exists
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_create_upload_url` with `fileName: "report.pdf"`, `folderId: "<id>"`
  - Then `detectedMimeType` is `application/pdf`
- **Priority:** High

#### E2E-015: Upload With Explicit MIME Type Override
- **Category:** Feature
- **Scenario:** SC-006
- **Requirements:** FR-010
- **Preconditions:** MCP session established; target folder exists
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_create_upload_url` with `fileName: "data.bin"`, `folderId: "<id>"`, `mimeType: "application/x-custom"`
  - Then `detectedMimeType` is `application/x-custom`
- **Priority:** High

#### E2E-016: Upload Updates Existing File
- **Category:** Feature
- **Scenario:** SC-006
- **Requirements:** FR-010
- **Preconditions:** MCP session established; file with known name already exists in target folder
- **Steps:**
  - Given an authenticated MCP session and a file "existing.txt" in folder X
  - When agent calls `drive_create_upload_url` with `fileName: "existing.txt"`, `folderId: "<X_id>"`
  - Then `isUpdate` is true
  - And `fileId` matches the existing file's ID
- **Priority:** High

#### E2E-017: Share With User Including Notification
- **Category:** Feature
- **Scenario:** SC-013
- **Requirements:** FR-017
- **Preconditions:** MCP session established; test file exists
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_permissions_update` with `action: "add"`, `type: "user"`, `email: "test@example.com"`, `role: "reader"`, `notify: true`, `message: "Check this out"`
  - Then response contains updated permissions including the new user entry
- **Priority:** High

#### E2E-018: Share Publicly
- **Category:** Feature
- **Scenario:** SC-013
- **Requirements:** FR-017
- **Preconditions:** MCP session established; test file exists
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_permissions_update` with `action: "add"`, `type: "anyone"`, `role: "reader"`
  - Then response contains an "anyone" permission with role "reader"
- **Priority:** High

#### E2E-019: Activity Deleted With Days Filter
- **Category:** Feature
- **Scenario:** SC-015
- **Requirements:** FR-019
- **Preconditions:** MCP session established; files were recently trashed
- **Steps:**
  - Given an authenticated MCP session and recently trashed files
  - When agent calls `drive_activity_deleted` with `daysBack: 1`
  - Then results only include files trashed within the last day
- **Priority:** Medium

#### E2E-020: Activity History Respects 200 Cap
- **Category:** Feature
- **Scenario:** SC-016
- **Requirements:** FR-020
- **Preconditions:** MCP session established; Drive has extensive activity
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_activity_history` with `maxResults: 500`
  - Then results contain at most 200 entries
- **Priority:** High

#### E2E-021: Invalid File ID Returns Clear Error
- **Category:** Error
- **Scenario:** SC-003
- **Requirements:** FR-007
- **Preconditions:** MCP session established
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_file_info` with `fileId: "nonexistent_id_12345"`
  - Then response is an MCP error with message containing "not found"
- **Priority:** Critical

#### E2E-022: Permission Denied on File Access
- **Category:** Error
- **Scenario:** SC-003
- **Requirements:** FR-007, FR-008
- **Preconditions:** MCP session established; file exists but user lacks access
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_file_info` with a `fileId` the user cannot access
  - Then response is an MCP error with message containing "Permission denied" or "not found"
- **Priority:** High

#### E2E-023: Export Non-Workspace File Returns Error
- **Category:** Error
- **Scenario:** SC-004, SC-005
- **Requirements:** FR-008, FR-009
- **Preconditions:** MCP session established; regular file (e.g., PDF) exists
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_export_url` with a non-Workspace `fileId` and `format: "pdf"`
  - Then response is an MCP error with message containing "Not a Google Workspace file"
  - When agent calls `drive_download_url` with a Google Docs `fileId`
  - Then response is an MCP error with message containing "Cannot download Google Workspace files"
- **Priority:** High

#### E2E-024: Export With Unsupported Format Returns Error
- **Category:** Error
- **Scenario:** SC-005
- **Requirements:** FR-009
- **Preconditions:** MCP session established; Google Sheets file exists
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_export_url` with a Sheets `fileId` and `format: "docx"`
  - Then response is an MCP error with message containing "not supported" and listing supported formats
- **Priority:** High

#### E2E-025: Create Folder With Invalid Parent ID
- **Category:** Error
- **Scenario:** SC-011
- **Requirements:** FR-015, FR-006, FR-010
- **Preconditions:** MCP session established
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_folder_create` with `parentFolderId: "nonexistent_id"` and `name: "test"`
  - Then response is an MCP error with message containing "not found"
- **Priority:** High

#### E2E-026: Delete Already-Trashed File
- **Category:** Error
- **Scenario:** SC-007
- **Requirements:** FR-011
- **Preconditions:** MCP session established; file is already in trash
- **Steps:**
  - Given an authenticated MCP session and a file already in trash
  - When agent calls `drive_delete` with the trashed file's `fileId`
  - Then response is an MCP error with message containing "already in trash"
- **Priority:** Medium

#### E2E-027: Move File to Non-Existent Folder
- **Category:** Error
- **Scenario:** SC-009
- **Requirements:** FR-013
- **Preconditions:** MCP session established; test file exists
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_move` with valid `fileId` and `targetFolderId: "nonexistent_id"`
  - Then response is an MCP error with message containing "not found"
- **Priority:** High

#### E2E-028: Copy Without Required Target Folder
- **Category:** Error
- **Scenario:** SC-010
- **Requirements:** FR-014
- **Preconditions:** MCP session established
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_copy` with `fileId` but without `targetFolderId`
  - Then response is an MCP error with message containing "targetFolderId is required"
- **Priority:** High

#### E2E-029: Remove Non-Existent Permission ID
- **Category:** Error
- **Scenario:** SC-013
- **Requirements:** FR-017
- **Preconditions:** MCP session established; test file exists
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_permissions_update` with `action: "remove"`, `permissionId: "nonexistent"`
  - Then response is an MCP error with message containing "not found"
- **Priority:** Medium

#### E2E-030: Expired OAuth Token Triggers 401
- **Category:** Error
- **Scenario:** SC-018
- **Requirements:** FR-003
- **Preconditions:** MCP server is running
- **Steps:**
  - Given a request with an expired Bearer token
  - When client sends a `tools/list` MCP request
  - Then HTTP response is 401
  - And `WWW-Authenticate` header contains `error="invalid_token"` and `resource_metadata` URL
- **Priority:** Critical

#### E2E-031: Missing Bearer Token Returns 401
- **Category:** Error
- **Scenario:** SC-018
- **Requirements:** FR-003
- **Preconditions:** MCP server is running
- **Steps:**
  - Given a request with no Authorization header
  - When client sends a `tools/list` MCP request to `/`
  - Then HTTP response is 401
  - And `WWW-Authenticate` header contains `resource_metadata` URL pointing to `/.well-known/oauth-protected-resource`
- **Priority:** Critical

#### E2E-032: Invalid Bearer Token Returns 401
- **Category:** Error
- **Scenario:** SC-018
- **Requirements:** FR-003
- **Preconditions:** MCP server is running
- **Steps:**
  - Given a request with `Authorization: Bearer invalid_garbage_token`
  - When client sends a `tools/list` MCP request
  - Then HTTP response is 401
  - And `WWW-Authenticate` header contains `error="invalid_token"`
- **Priority:** Critical

#### E2E-033: OAuth Consent Denied by User
- **Category:** Error
- **Scenario:** SC-018
- **Requirements:** FR-002
- **Preconditions:** MCP server is running; OAuth flow initiated
- **Steps:**
  - Given a valid authorization state
  - When Google redirects to `/oauth/callback` with `error=access_denied`
  - Then server returns HTTP 400 with error message containing "access_denied"
- **Priority:** High

#### E2E-034: OAuth State Expired
- **Category:** Error
- **Scenario:** SC-018
- **Requirements:** FR-002
- **Preconditions:** MCP server is running
- **Steps:**
  - Given an authorization state that was created more than 10 minutes ago
  - When Google redirects to `/oauth/callback` with the expired state
  - Then server returns HTTP 400 with error message containing "Invalid or expired state"
- **Priority:** High

#### E2E-035: Search Returns Zero Results
- **Category:** Edge
- **Scenario:** SC-001
- **Requirements:** FR-005
- **Preconditions:** MCP session established
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_search` with `query: "zzz_nonexistent_file_xyz_12345"`
  - Then response contains an empty array (not null)
  - And no error is returned
- **Priority:** Medium

#### E2E-036: List Empty Folder
- **Category:** Edge
- **Scenario:** SC-002
- **Requirements:** FR-006
- **Preconditions:** MCP session established; empty folder exists
- **Steps:**
  - Given an authenticated MCP session and an empty folder
  - When agent calls `drive_folder_list` with the empty folder's `id`
  - Then response contains an empty array (not null)
  - And no error is returned
- **Priority:** Medium

#### E2E-037: File Info on Root Folder
- **Category:** Edge
- **Scenario:** SC-003
- **Requirements:** FR-007
- **Preconditions:** MCP session established
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_file_info` with `fileId: "root"`
  - Then response contains metadata for My Drive root
  - And `path` is empty or contains single root entry
- **Priority:** Medium

#### E2E-038: Search With Special Characters in Query
- **Category:** Edge
- **Scenario:** SC-001
- **Requirements:** FR-005
- **Preconditions:** MCP session established
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_search` with `query: "file's \"name\" & (test)"`
  - Then the query is properly escaped and does not cause an API error
  - And response is a valid array (possibly empty)
- **Priority:** Medium

#### E2E-039: Folder Name With Unicode Characters
- **Category:** Edge
- **Scenario:** SC-011
- **Requirements:** FR-015
- **Preconditions:** MCP session established; parent folder exists
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_folder_create` with `name: "Dossier Resume 2026"`
  - Then folder is created successfully
  - And response `name` exactly matches the Unicode input
- **Priority:** Medium

#### E2E-040: Very Long Filename in Upload
- **Category:** Edge
- **Scenario:** SC-006
- **Requirements:** FR-010
- **Preconditions:** MCP session established; target folder exists
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_create_upload_url` with a `fileName` of 300 characters
  - Then either the session is created successfully (Google accepts long names) or a clear error is returned
- **Priority:** Low

#### E2E-041: Max Results Boundary Values
- **Category:** Edge
- **Scenario:** SC-001
- **Requirements:** FR-005
- **Preconditions:** MCP session established
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_search` with `maxResults: 1`
  - Then at most 1 result is returned
  - When agent calls `drive_search` with `maxResults: 0`
  - Then system uses default (50) or returns validation error
- **Priority:** Medium

#### E2E-042: Activity History With Zero Days Back
- **Category:** Edge
- **Scenario:** SC-016
- **Requirements:** FR-020
- **Preconditions:** MCP session established
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_activity_history` with `daysBack: 0`
  - Then response is empty or contains only very recent activity
- **Priority:** Low

#### E2E-043: Revisions on File With No Revision History
- **Category:** Edge
- **Scenario:** SC-017
- **Requirements:** FR-021
- **Preconditions:** MCP session established
- **Steps:**
  - Given an authenticated MCP session and a newly created Google Workspace file
  - When agent calls `drive_file_revisions` with the file's `id`
  - Then response is an empty array or contains a single initial revision
  - And no error is returned
- **Priority:** Low

#### E2E-044: Concurrent Requests With Different User Tokens
- **Category:** Edge
- **Scenario:** SC-018
- **Requirements:** FR-003, FR-004
- **Preconditions:** MCP server is running; two valid Bearer tokens from different users
- **Steps:**
  - Given two valid but different Bearer tokens
  - When both tokens send `drive_search` concurrently
  - Then each request returns results from the respective user's Drive
  - And no cross-contamination of data occurs
- **Priority:** High

#### E2E-045: Protected Resource Metadata Serves Correct JSON
- **Category:** Security
- **Scenario:** SC-018
- **Requirements:** FR-002
- **Preconditions:** MCP server is running
- **Steps:**
  - Given the MCP server is running
  - When client sends `GET /.well-known/oauth-protected-resource`
  - Then response is JSON with `resource`, `authorization_servers`, `bearer_methods_supported`
  - And `authorization_servers` contains the server's base URL
- **Priority:** Critical

#### E2E-046: Authorization Server Metadata Serves Correct JSON
- **Category:** Security
- **Scenario:** SC-018
- **Requirements:** FR-002
- **Preconditions:** MCP server is running
- **Steps:**
  - Given the MCP server is running
  - When client sends `GET /.well-known/oauth-authorization-server`
  - Then response is JSON with `issuer`, `authorization_endpoint`, `token_endpoint`, `registration_endpoint`
  - And `code_challenge_methods_supported` contains `S256`
  - And `grant_types_supported` contains `authorization_code` and `refresh_token`
- **Priority:** Critical

#### E2E-047: Dynamic Client Registration Returns Credentials
- **Category:** Security
- **Scenario:** SC-018
- **Requirements:** FR-002
- **Preconditions:** MCP server is running
- **Steps:**
  - Given the MCP server is running
  - When client sends `POST /oauth/register` with `{"redirect_uris": ["http://localhost:3000/callback"]}`
  - Then response is HTTP 201 with `client_id` and `client_secret`
  - And `redirect_uris` matches the request
- **Priority:** High

#### E2E-048: PKCE S256 Validation Succeeds With Correct Verifier
- **Category:** Security
- **Scenario:** SC-018
- **Requirements:** FR-002
- **Preconditions:** Authorization code exists with stored code_challenge
- **Steps:**
  - Given a stored authorization code with `code_challenge = BASE64URL(SHA256(verifier))`
  - When client sends token request with the correct `code_verifier`
  - Then PKCE validation succeeds
  - And tokens are returned
- **Priority:** Critical

#### E2E-049: PKCE S256 Validation Fails With Wrong Verifier
- **Category:** Security
- **Scenario:** SC-018
- **Requirements:** FR-002
- **Preconditions:** Authorization code exists with stored code_challenge
- **Steps:**
  - Given a stored authorization code with a code_challenge
  - When client sends token request with an incorrect `code_verifier`
  - Then PKCE validation fails
  - And response is an OAuth error with `error: "invalid_grant"`
- **Priority:** Critical

#### E2E-050: Health Endpoint Responds Within 100ms
- **Category:** Performance
- **Scenario:** SC-020
- **Requirements:** FR-022
- **Preconditions:** MCP server is running
- **Steps:**
  - Given the MCP server is running
  - When client sends `GET /health`
  - Then response is HTTP 200 with body "OK"
  - And response time is under 100ms
- **Priority:** High

#### E2E-051: MCP Initialize and Tools List Within 2s
- **Category:** Performance
- **Scenario:** SC-001
- **Requirements:** FR-001, FR-025
- **Preconditions:** MCP server is running
- **Steps:**
  - Given the MCP server is running
  - When client sends `initialize` followed by `tools/list`
  - Then both complete within 2 seconds total
  - And `tools/list` returns 20 tools (including ping)
- **Priority:** High

#### E2E-052: Activity History With 200 Cap Completes Within Timeout
- **Category:** Performance
- **Scenario:** SC-016
- **Requirements:** FR-020
- **Preconditions:** MCP session established; Drive has extensive activity
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_activity_history` with `maxResults: 200`, `daysBack: 30`
  - Then response completes within 300 seconds
  - And results contain at most 200 entries
- **Priority:** Medium

#### E2E-053: Search Then Info Then Download (Chained Workflow)
- **Category:** Cross-Scenario
- **Scenario:** SC-001, SC-003, SC-004
- **Requirements:** FR-005, FR-007, FR-008
- **Preconditions:** MCP session established; test file exists
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_search` with a query matching a known file
  - And extracts the `id` from the first result
  - And calls `drive_file_info` with that `id`
  - And calls `drive_download_url` with that `id`
  - Then all three calls succeed
  - And the file info and download URL reference the same file
- **Priority:** High

#### E2E-054: Create Folder Then Upload Then List (Upload Workflow)
- **Category:** Cross-Scenario
- **Scenario:** SC-011, SC-006, SC-002
- **Requirements:** FR-015, FR-010, FR-006
- **Preconditions:** MCP session established; parent folder exists
- **Steps:**
  - Given an authenticated MCP session
  - When agent calls `drive_folder_create` to create "upload-test" folder
  - And calls `drive_create_upload_url` with `fileName: "test.txt"` targeting the new folder
  - And calls `drive_folder_list` on the new folder
  - Then folder creation succeeds
  - And upload URL is created for the new folder
  - And folder list reflects the expected state
- **Priority:** High

#### E2E-055: Share Then List Then Remove (Permission Workflow)
- **Category:** Cross-Scenario
- **Scenario:** SC-013, SC-012
- **Requirements:** FR-017, FR-016
- **Preconditions:** MCP session established; test file exists
- **Steps:**
  - Given an authenticated MCP session and a test file
  - When agent calls `drive_permissions_update` to add "anyone" reader permission
  - And calls `drive_permissions_list` on the file
  - Then the "anyone" permission appears in the list
  - When agent extracts the permission ID and calls `drive_permissions_update` to remove it
  - And calls `drive_permissions_list` again
  - Then the "anyone" permission is no longer in the list
- **Priority:** High

## 11. Implementation Phases

### Phase 1: Core MCP Server + Auth (Foundation)
**Goal:** MCP server starts, handles sessions, serves OAuth2 endpoints, authenticates requests

**Deliverables:**
1. Refactor `internal/auth/` to add context injection (`WithOAuthConfig`, `WithAccessToken`, `GetOAuthConfigFromContext`, `GetAccessTokenFromContext`)
2. Create `internal/mcp/server.go` -- MCP server setup, HTTP handler, auth middleware, health endpoint
3. Create `internal/mcp/oauth2.go` -- OAuth2 authorization server (RFC 8414/9728/7591, PKCE S256)
4. Create `internal/mcp/templates/success.html` -- OAuth success page
5. Add `mcp` subcommand to `cmd/gdrive/main.go` via `internal/cli/cli.go`
6. Register `ping` tool only
7. Structured logging with `slog`
8. Add `go.sum`/`go.mod` dependencies (`modelcontextprotocol/go-sdk`, `cloud.google.com/go/secretmanager`, `google.golang.org/api`)

**Testable:** E2E-030, E2E-031, E2E-032, E2E-045, E2E-046, E2E-047, E2E-048, E2E-049, E2E-050, E2E-051 (with ping only)

### Phase 2: Drive Tools -- Read Operations
**Goal:** All read-only Drive tools working

**Deliverables:**
1. Create `internal/mcp/tools.go` -- tool registration and handler implementations
2. Implement: `drive_search`, `drive_folder_list`, `drive_file_info`, `drive_download_url`, `drive_export_url`, `drive_file_revisions`, `drive_permissions_list`
3. Implement: `drive_activity_changes`, `drive_activity_deleted`, `drive_activity_history`
4. Input/output struct definitions with `jsonschema` tags

**Testable:** E2E-001 through E2E-005, E2E-010 through E2E-013, E2E-019 through E2E-024, E2E-035 through E2E-038, E2E-041 through E2E-043, E2E-053

### Phase 3: Drive Tools -- Write Operations
**Goal:** All mutating Drive tools working

**Deliverables:**
1. Implement: `drive_create_upload_url`, `drive_delete`, `drive_rename`, `drive_move`, `drive_copy`, `drive_folder_create`, `drive_permissions_update`
2. MIME type auto-detection for uploads
3. Existing-file detection for update semantics

**Testable:** E2E-006 through E2E-009, E2E-014 through E2E-018, E2E-025 through E2E-029, E2E-039, E2E-040, E2E-044, E2E-052, E2E-054, E2E-055

### Phase 4: Infrastructure + Deployment
**Goal:** Terraform-managed deployment to Cloud Run with custom domain

**Deliverables:**
1. Create `config.yaml` (GCP project, region, resources, secrets)
2. Create `Dockerfile` (multi-stage Go build)
3. Create `init/` Terraform (state backend, service accounts, API enablement)
4. Create `iac/` Terraform (Cloud Run, Firestore, Secret Manager, Artifact Registry, Cloud DNS, domain mapping)
5. Update `Makefile` with 6 infrastructure targets
6. Create initial OAuth credentials in Secret Manager (manual step)

**Testable:** SC-019 (manual verification), SC-020 (health check against deployed instance)

### Phase 5: Documentation + Polish
**Goal:** Complete documentation and production readiness

**Deliverables:**
1. Update `README.md` with MCP server usage and deployment guide
2. Update `CLAUDE.md` with MCP architecture
3. Create `.agent_docs/mcp-server.md`, `.agent_docs/terraform.md`, `.agent_docs/authentication.md`
4. Claude Desktop skill file (optional)
5. Final review and testing against deployed instance

## 12. Open Questions & TBDs

| ID | Question | Context |
|----|----------|---------|
| TBD-001 | What is the GCP project ID for the gdrive MCP deployment? | Needed for `config.yaml` before deployment |
| TBD-002 | What prefix should be used for GCP resource naming? (e.g., `scmgdrive`) | Follows google-contacts pattern of `scmgcontacts` |
| TBD-003 | Should the Cloud DNS zone for `scm-platform.org` be created by this project or does it already exist? | If shared across multiple MCP servers, the zone may already exist |
| TBD-004 | What Google OAuth app name and consent screen text should be used? | Separate from google-contacts' OAuth app |
| TBD-005 | Should the Firestore `api_keys` collection be in the default database or a named database? | google-contacts uses `(default)` |

## 13. Glossary

| Term | Definition |
|------|-----------|
| MCP | Model Context Protocol -- open standard for AI agent tool communication |
| HTTP Streamable | MCP transport over HTTP with session tracking via `Mcp-Session-Id` header |
| Bearer Token | OAuth2 access token sent in `Authorization: Bearer <token>` header |
| PKCE | Proof Key for Code Exchange -- prevents authorization code interception |
| S256 | SHA-256 based PKCE code challenge method |
| Resumable Upload | Google Drive upload protocol that supports chunked/resumable file uploads |
| Workspace File | Google Docs, Sheets, or Slides file (native format, requires export for download) |
| Soft Delete | Moving a file to Google Drive trash (recoverable) vs permanent deletion |
| Cloud Run | Google Cloud serverless container platform |
| Firestore | Google Cloud NoSQL document database |
| Secret Manager | Google Cloud service for storing sensitive configuration |
