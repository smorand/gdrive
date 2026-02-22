# MCP Server Documentation

## Overview

The MCP (Model Context Protocol) HTTP Streamable server exposes Google Drive operations as 20 MCP tools for AI agents. It runs as a `gdrive mcp` subcommand and deploys to Cloud Run.

## Architecture

```
Client (AI Agent)
    │
    ▼ HTTP + Bearer Token
┌─────────────────────────────────────────┐
│  HTTP Mux                               │
│  ├── GET /health (no auth)              │
│  ├── GET /.well-known/oauth-*           │
│  ├── POST /oauth/register               │
│  ├── GET /oauth/authorize               │
│  ├── GET /oauth/callback                │
│  ├── POST /oauth/token                  │
│  └── /mcp (auth middleware)             │
│       └── StreamableHTTP Server         │
│            └── MCP Tools (20)           │
└─────────────────────────────────────────┘
```

## Package Structure

- `internal/mcp/server.go` - Server core, HTTP mux, auth middleware, health endpoint
- `internal/mcp/oauth2.go` - OAuth2 authorization server (RFC 8414/9728/7591, PKCE S256)
- `internal/mcp/tools.go` - All 20 MCP tools (read + write)
- `internal/cli/mcp.go` - Cobra CLI subcommand

## MCP Tools (20 total)

### Read Tools (registered via `RegisterReadTools`)

| Tool | Description | Key Inputs |
|------|-------------|------------|
| `drive_search` | Search files across Drive | `query`, `maxResults` |
| `drive_folder_list` | List folder contents | `folderId` |
| `drive_file_info` | Get file metadata with path | `fileId` |
| `drive_download_url` | Get signed download URL | `fileId` |
| `drive_export_url` | Get export URL for Workspace files | `fileId`, `format` |
| `drive_activity_changes` | List recent changes | `maxResults` |
| `drive_activity_deleted` | List trashed files | `daysBack`, `maxResults` |
| `drive_activity_history` | Query Drive Activity API | `daysBack`, `maxResults` (cap 200) |
| `drive_file_revisions` | List file revision history | `fileId` |

### Write Tools (registered via `RegisterWriteTools`)

| Tool | Description | Key Inputs |
|------|-------------|------------|
| `drive_delete` | Move file to trash | `fileId` |
| `drive_rename` | Rename a file | `fileId`, `newName` |
| `drive_move` | Move file to folder | `fileId`, `targetFolderId` |
| `drive_copy` | Copy a file | `fileId`, `targetFolderId`, `newName` |
| `drive_folder_create` | Create a folder | `parentFolderId`, `name` |
| `drive_permissions_list` | List permissions | `fileId` |
| `drive_permissions_update` | Add/remove permissions | `fileId`, `action`, `type`, `role`, `email`, `permissionId` |
| `drive_create_upload_url` | Get resumable upload URL | `folderId`, `fileName`, `mimeType` |

Plus `ping` (registered in server.go).

## Key Design Decisions

- **ID-only parameters**: All tools use Google Drive IDs, no path resolution
- **Signed URLs**: File transfers return URLs instead of streaming content
- **Soft delete only**: `drive_delete` trashes files, no permanent deletion
- **Per-request auth**: Each request creates its own Drive client from context-injected token
- **Activity cap**: `drive_activity_history` hard caps at 200 results to prevent Cloud Run timeout

## Auth Flow

1. Client discovers OAuth metadata via `/.well-known/oauth-protected-resource`
2. Client registers dynamically via `POST /oauth/register`
3. Client starts authorization via `GET /oauth/authorize` (PKCE S256 required)
4. User authenticates with Google OAuth
5. Server receives callback, exchanges code, stores tokens
6. Client exchanges authorization code for access token via `POST /oauth/token`
7. Client uses Bearer token for all MCP requests

## Configuration

### CLI Flags (with environment variable fallbacks)

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--port` | `PORT` | 8080 | Server port |
| `--host` | `HOST` | 0.0.0.0 | Server host |
| `--base-url` | `BASE_URL` | http://localhost:{port} | External base URL |
| `--secret-name` | `SECRET_NAME` | - | GCP Secret Manager secret name |
| `--secret-project` | `SECRET_PROJECT` | - | GCP project ID |
| `--credential-file` | `CREDENTIAL_FILE` | - | Local OAuth credentials file |

### Logging

- `ENVIRONMENT=prd` → JSON format (slog)
- Otherwise → text format
- `LOG_LEVEL` → debug/info/warn/error (default: info)

## Helper Functions in tools.go

- `getDriveService(ctx)` - Creates authenticated Drive service from context
- `getActivityService(ctx)` - Creates authenticated Activity service from context
- `toolResult(data)` - JSON-marshals data into MCP CallToolResult
- `logToolCall(name, start, result, err)` - Logs tool execution with duration
- `detectMimeType(filename)` - Auto-detects MIME type from file extension
