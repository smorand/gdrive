# CLAUDE.md - AI Assistant Guide for gdrive

## Project Overview

A high-performance command-line tool and MCP server for Google Drive operations, built in Go. Provides comprehensive file and folder management with parallel downloads, progress tracking, smart path resolution, and flexible configuration management. Includes an MCP HTTP Streamable server that exposes all Drive operations as 20 tools for AI agents.

## Project Structure

This project follows the **Standard Go Project Layout**:

```
gdrive/
├── cmd/gdrive/main.go        # Minimal entry point
├── internal/
│   ├── auth/                  # OAuth2 authentication (CLI + MCP context injection)
│   ├── cli/                   # CLI commands (file, folder, search, activity, mcp)
│   ├── drive/                 # Drive API operations (service.go, activity.go)
│   └── mcp/                   # MCP server (server.go, oauth2.go, tools.go)
├── init/                      # Terraform: state backend, service accounts, APIs
├── iac/                       # Terraform: Cloud Run, DNS, Docker, secrets
├── .agent_docs/               # Detailed documentation (loaded on demand)
├── config.yaml                # Infrastructure single source of truth
├── Dockerfile                 # Multi-stage Go build for Cloud Run
├── Makefile                   # Build + infrastructure automation
├── CLAUDE.md                  # This file
└── README.md                  # User documentation
```

**Note:** This project does NOT use a `src/` directory, following Go best practices.

## Architecture

### Core Components

1. **internal/auth/auth.go** - Authentication module
   - Handles OAuth2 flow with Google Drive API (CLI mode)
   - Context-based token injection for MCP mode (`WithOAuthConfig`, `WithAccessToken`)
   - Configuration priority: CLI flags > Environment variables > Defaults
   - `GetAuthenticatedServiceWithContext(ctx)` for per-request MCP auth
   - See `.agent_docs/authentication.md` for full details

2. **internal/drive/service.go** - Drive operations service
   - `Service` struct encapsulates all Drive API interactions
   - Path resolution: converts human paths to Google Drive folder IDs
   - File operations: upload, download with progress tracking
   - Folder operations: recursive traversal and sync
   - List and search operations: query Drive API for file metadata
   - Comprehensive MIME type mappings for all common file types
   - Exports: `MIMETypeMappings`, `ExportFormats` for MIME type handling

3. **internal/drive/activity.go** - Activity tracking
   - `ChangeInfo` and `RevisionInfo` structs for activity data
   - `DriveActivityInfo` for comprehensive activity history
   - Functions: `ListChanges()`, `ListTrashedFiles()`, `ListRevisions()`, `QueryDriveActivity()`

4. **internal/cli/cli.go** - Command-line interface
   - Built with Cobra framework for robust CLI
   - Four command groups: `file`, `folder`, `search`, and `activity`
   - fatih/color library for colored terminal output
   - progressbar/v3 library for real-time progress tracking
   - Explicit initialization via constructor functions (no `init()`)
   - `SetupRootCommand()`: Configures global flags and pre-run hook
   - `FileCmd()`, `FolderCmd()`, `SearchCmd()`, `ActivityCmd()`: Command constructors

5. **cmd/gdrive/main.go** - Minimal entry point
   - Creates root Cobra command
   - Calls `cli.SetupRootCommand()` for global configuration
   - Adds subcommands via constructor functions (file, folder, search, activity, mcp)
   - Handles execution and error output

6. **internal/mcp/server.go** - MCP HTTP Streamable server
   - HTTP mux with health, OAuth2, and MCP endpoints
   - Auth middleware enforces Bearer token with WWW-Authenticate headers
   - `httpContextFunc` injects auth context from HTTP request into MCP context
   - Graceful shutdown on SIGINT/SIGTERM
   - Structured logging via `slog` (JSON in prd, text otherwise)

7. **internal/mcp/oauth2.go** - OAuth2 authorization server
   - RFC 8414/9728/7591 compliant with PKCE S256
   - Proxies to Google OAuth for user authentication
   - Dynamic client registration, in-memory state stores
   - Credential loading: Secret Manager → local file fallback
   - See `.agent_docs/authentication.md` for full flow

8. **internal/mcp/tools.go** - 20 MCP tools for Google Drive
   - 9 read tools + 8 write tools + ping
   - All tools use ID-only parameters (no path resolution)
   - Signed URLs for file transfers
   - See `.agent_docs/mcp-server.md` for full tool reference

9. **Infrastructure** (init/, iac/, config.yaml, Dockerfile)
   - Three-phase Terraform deployment to Cloud Run
   - Custom domain: `drive.mcp.scm-platform.org`
   - See `.agent_docs/terraform.md` for full details

### Key Design Patterns

#### Configuration Management
- **Priority System**: CLI flags > Environment variables > Default values
- **Config Structure**:
  - `Config` struct holds `ConfigDir` and `CredentialsPath`
  - `NewConfig(cliConfigDir, cliCredentialsPath)` resolves configuration
  - Methods: `GetConfigDir()`, `GetTokenPath()`, `GetCredentialsPath()`
- **Global State**:
  - `globalConfig` variable initialized in `PersistentPreRun` hook
  - Available to all commands via `getDriveService()`
- **Environment Variables**:
  - `GDRIVE_CONFIG_DIR`: Override config directory (default: `~/.gdrive`)
  - `GDRIVE_CREDENTIALS_PATH`: Override credentials file path
- **Default Behavior**:
  - Config directory: `$HOME/.gdrive`
  - Credentials lookup: Current directory → Config directory
  - Token storage: `{ConfigDir}/token.json`

#### Path Resolution
- Human-friendly paths like `Parameters/bin` are resolved to Drive folder IDs
- Resolution happens recursively: root → Parameters → bin
- Efficient caching to avoid redundant API calls
- All commands support `--id` flag to bypass path resolution and use IDs directly

#### ID Support
- All file and folder commands accept `--id` flag
- When `--id` is used, the path/file argument is treated as a Google Drive ID
- Useful for working with shared files/folders or avoiding ambiguity

#### File Versioning
- Google Drive native versioning is used
- Uploading to an existing file creates a new version
- No manual version tracking required

#### Progress Display
- progressbar/v3 library provides real-time progress bars
- Transfer speed, ETA, and file size shown during operations
- Clean, informative output with colors

#### Error Handling
- Standard Go error handling with proper error wrapping
- All errors properly returned up the call stack
- User-facing errors displayed with color coding

## Implementation Details

### Configuration Resolution Flow

```
1. Parse CLI flags (--config-dir, --credentials)
2. Check environment variables (GDRIVE_CONFIG_DIR, GDRIVE_CREDENTIALS_PATH)
3. Fall back to defaults (~/.gdrive, ./credentials.json)
4. Create Config struct with resolved paths
5. Store in globalConfig variable (available to all commands)
```

### Authentication Flow

```
1. Resolve config paths using NewConfig()
2. Look for credentials.json using GetCredentialsPath()
3. Check for existing token at Config.GetTokenPath()
4. If valid, use it
5. If expired, OAuth2 client handles refresh automatically
6. If none, start OAuth2 flow
7. Save new token to Config.GetTokenPath()
```

### File Upload Logic

```
1. Resolve target folder path to ID
2. Check if file exists in target folder
3. If exists: update file (new version)
4. If not: create new file
5. Use io.TeeReader for progress tracking
```

### Folder Upload Logic

```
1. Verify local folder exists
2. Resolve remote parent folder (must exist)
3. For each item in local folder:
   - If file: upload it
   - If folder:
     - Create on Drive if doesn't exist
     - Recurse into it
```

### Download with Timestamp Preservation

```
1. Download file to local filesystem
2. Extract modifiedTime from Drive metadata
3. Parse RFC3339 timestamp
4. Use os.Chtimes() to set local file timestamp
```

### Parallel Folder Downloads

```
1. List all items in folder
2. Process folders first (sequential, recursive)
3. Collect all files to download
4. Create semaphore channel with size = parallel flag (default 5)
5. Spawn goroutines for each file download
6. Each goroutine:
   - Acquires semaphore slot
   - Downloads file
   - Releases semaphore slot
7. Wait for all downloads to complete (WaitGroup)
8. Collect and return any errors
```

**Implementation details:**
- Semaphore pattern using buffered channel limits concurrent downloads
- WaitGroup ensures all downloads complete before returning
- Mutex-protected error slice collects errors from goroutines
- Configurable via `--parallel` flag (1-20, default: 5)
- Folders are processed sequentially to ensure structure exists before file downloads

### List and Search Display

```
1. Query Drive API with filters
2. For search: expand file type shortcuts to MIME types
3. Receive list of items with metadata
4. Format data into table with aligned columns
5. Sort (list only): folders first, then files
6. Display human-readable sizes
```

### Google Workspace Export

```
1. Detect Google Workspace file via MIME type
2. Determine default export format (PDF/DOCX/XLSX/PPTX)
3. Get export MIME type from format mapping
4. Use Files.Export() API instead of Files.Get()
5. Adjust local filename extension based on export format
6. Download and save with proper extension
```

### Permissions Management

```
1. ShareFile: Create user/group permission with role and notification
2. ShareWithAnyone: Create "anyone" permission for public link sharing
3. ListPermissions: Get all permissions with user/group/domain/anyone details
4. RemovePermission: Delete specific permission by ID
5. RemovePublicAccess: Find and remove all "anyone" permissions
6. All operations support Shared Drives via SupportsAllDrives(true)
```

### File Operations

```
1. DeleteFile: Remove file/folder from Drive (moves to trash)
2. RenameFile: Update file name while preserving location
3. MoveFile: Change parent folder (removes old parents, adds new)
4. CopyFile: Create duplicate with optional new name/location
5. GetFilePath: Traverse parent folders to reconstruct full path
6. GetFileInfo: Complete metadata including owners, dates, path
```

### Activity Tracking

```
1. ListChanges: Get recent changes to files in Drive
   - Uses Changes.GetStartPageToken() for current state
   - Returns ChangeInfo with file name, type, modified by, time
   - Change types: Added, Modified, Removed (color-coded in display)
   - Configurable page size (default: 50)

2. ListTrashedFiles: Get recently deleted files (in trash)
   - Query: "trashed = true" with optional time filter
   - Filters by trashedTime >= cutoff date
   - Returns file name, deletion time, size, and who deleted it
   - Ordered by trashedTime descending (newest first)
   - Configurable days back (default: 7) and max results (default: 100)

3. QueryDriveActivity: Comprehensive activity history via Drive Activity API
   - Uses Drive Activity API v2 for complete activity logs
   - Includes permanent deletions, edits, moves, permission changes
   - Parses multiple action types: Create, Edit, Move, Rename, Delete, Restore, Permission
   - Distinguishes between trash (TRASH) and permanent deletion (PERMANENT_DELETE)
   - Returns DriveActivityInfo with timestamp, actors, targets, action details
   - Configurable time filter (default: 7 days) and page size (default: 100)

4. ListRevisions: Get revision history for a specific file
   - Uses Revisions.List() API endpoint
   - Returns RevisionInfo with modification time, size, author
   - Shows keepForever and published status
   - Displays in reverse chronological order (newest first)
   - Note: May be incomplete for files with large revision history

5. GetRevision: Get specific revision metadata
   - Used internally for detailed revision information
```

## Technology Stack

### Language & Framework
- **Language**: Go 1.21+
- **CLI Framework**: Cobra for command-line interface
- **Color Output**: fatih/color for terminal colors
- **Progress Bars**: progressbar/v3 for transfer tracking
- **Build**: Statically-linked compiled binary

### Performance Characteristics
- **Startup**: Instant execution (compiled binary)
- **I/O Operations**: Highly efficient with buffered streams
- **Memory**: Optimized with garbage collection
- **Concurrency**: Native goroutines for parallel operations

### Code Architecture
- **Type System**: Static typing with compile-time safety
- **Error Handling**: Explicit error returns and wrapping
- **Package System**: Go modules for dependency management

## Build and Deployment

### Build with Makefile

```bash
# Build for current platform
make build

# Build for all platforms (Linux, macOS Intel, macOS ARM)
make build-all

# Clean and rebuild
make rebuild

# Install to /usr/local/bin
make install

# Run tests
make test

# Format code
make fmt

# Run linter
make vet
```

### MCP Server

```bash
# Start locally
gdrive mcp --port 8080 --credential-file credentials.json

# Deploy to Cloud Run
make init-plan && make init-deploy    # First time: bootstrap
make plan && make deploy              # Deploy application
```

### Dependencies
All dependencies are declared in `go.mod` (at project root):
- `github.com/fatih/color` - Terminal colors
- `github.com/schollz/progressbar/v3` - Progress bars
- `github.com/spf13/cobra` - CLI framework
- `github.com/mark3labs/mcp-go` - MCP SDK (HTTP Streamable)
- `github.com/google/uuid` - UUID generation
- `cloud.google.com/go/secretmanager` - GCP Secret Manager
- `golang.org/x/oauth2` - OAuth2 authentication
- `google.golang.org/api` - Google API client

### Binary Output
- Binary location: `bin/gdrive-{os}-{arch}` (e.g., `bin/gdrive-linux-amd64`)
- Install with `make install` to `/usr/local/bin/`
- No runtime dependencies (statically linked)

## Common Modifications

### Adding a New Command

1. Create command in `internal/cli/cli.go`:
```go
func newSubCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "new-command ARG",
        Short: "Description",
        Args:  cobra.ExactArgs(1),
        RunE:  runNewCommand,
    }
    cmd.Flags().BoolP("flag", "f", false, "Flag description")
    return cmd
}

func runNewCommand(cmd *cobra.Command, args []string) error {
    // Implementation
    return nil
}
```

2. Register in parent command constructor (e.g., `FileCmd()`):
```go
func FileCmd() *cobra.Command {
    cmd := &cobra.Command{...}
    cmd.AddCommand(newSubCmd())
    return cmd
}
```

3. Add method to `drive.Service` if needed (in `internal/drive/service.go`)
4. Update README.md and CLAUDE.md
5. Rebuild with `make build`

**Note:** Do NOT use `init()` functions. Use explicit constructor functions instead.

### Adding New Drive API Operations

1. Add method to `Service` struct in `internal/drive/service.go`
2. Follow existing patterns: error handling, progress display
3. Use `ds.srv.Files` for API calls
4. Return structured data (pointer, slice, error)

## Testing Strategy

### Build Verification
```bash
make build
./bin/gdrive-linux-amd64 --help
./bin/gdrive-linux-amd64 file --help
./bin/gdrive-linux-amd64 folder --help
./bin/gdrive-linux-amd64 search --help
```

### Run Unit Tests
```bash
make test
```

### Functional Testing
Test key operations:
- File upload/download
- File management (delete, rename, move, copy)
- File info with path reconstruction
- Permissions (share, list, remove)
- Google Workspace export
- Folder upload/download with `--parallel` flag
- Search with type filters
- Path resolution vs. `--id` flag
- `--new-only` flag behavior
- Overwrite protection

## Go-Specific Considerations

### Error Handling
- Always check and return errors
- Use `fmt.Errorf()` for error wrapping
- Return errors from `RunE` functions for Cobra

### Memory Management
- No explicit cleanup needed (garbage collected)
- Close file handles with `defer`
- Close HTTP response bodies with `defer`

### String Handling
- Use `strings` package for manipulation
- No string interpolation - use `fmt.Sprintf()`
- Concatenation with `+` or `strings.Join()`

### File I/O
- Use `os` and `filepath` packages
- `filepath.Join()` for cross-platform paths
- `os.Stat()` for file metadata

## Maintenance

### Updating Dependencies
```bash
go get -u ./...
go mod tidy
```

### Code Formatting
```bash
make fmt
# Or manually:
gofmt -w ./cmd ./internal
```

### Linting
```bash
make vet
# Or manually:
go vet ./...
golangci-lint run  # if installed
```

## Key Advantages

1. **Parallel Downloads**: Native concurrency with goroutines enables downloading multiple files simultaneously (configurable 1-20 concurrent downloads)
2. **Single Binary**: No runtime dependencies - just copy and run
3. **Instant Startup**: Compiled binary starts immediately
4. **Cross-Platform**: Easy to build for Linux, macOS, Windows
5. **Type Safety**: Compile-time type checking prevents errors
6. **High Performance**: Optimized I/O and memory management
7. **Easy Deployment**: No runtime dependencies, package managers, or interpreters required

### Performance Benchmarks

| Operation | Sequential | --parallel 5 | --parallel 10 |
|-----------|-----------|--------------|----------------|
| 100 small files | ~50s | ~15s | ~10s |
| 10 large files (1GB) | ~180s | ~45s | ~25s |
| Single file | ~1s | ~1s | ~1s |

*Times are approximate and depend on file sizes, network speed, and API rate limits*

## Feature Matrix

| Feature | Status | Description |
|---------|--------|-------------|
| `file download` | ✅ | Download single file with progress |
| `file upload` | ✅ | Upload with versioning support |
| `file delete` | ✅ | Delete files with confirmation |
| `file rename` | ✅ | Rename files on Drive |
| `file move` | ✅ | Move files between folders |
| `file copy` | ✅ | Copy files with optional rename |
| `file info` | ✅ | Detailed file info with path |
| `file share` | ✅ | Share files with users |
| `file share-public` | ✅ | Share with anyone via link |
| `file permissions` | ✅ | List all file permissions |
| `file remove-permission` | ✅ | Remove specific permissions |
| `file remove-public` | ✅ | Remove public access |
| `folder create` | ✅ | Create nested folder paths |
| `folder upload` | ✅ | Recursive folder upload |
| `folder download` | ✅ | Parallel recursive download |
| `folder list` | ✅ | Display folder contents |
| `search` | ✅ | Search with MIME type filters |
| `activity changes` | ✅ | List recent Drive changes |
| `activity deleted` | ✅ | List recently deleted files (trash) |
| `activity history` | ✅ | Comprehensive activity (incl. permanent deletions) |
| `activity revisions` | ✅ | View file revision history |
| `--id` flag | ✅ | Direct ID support |
| `--overwrite` flag | ✅ | Skip overwrite prompts |
| `--parallel` flag | ✅ | Configurable concurrency (1-20) |
| `--new-only` flag | ✅ | Skip unchanged files |
| `--type` filter | ✅ | MIME type shortcuts |
| Progress bars | ✅ | Real-time transfer status |
| Timestamp preservation | ✅ | Maintain modification times |
| Google Workspace export | ✅ | Auto-export to PDF/DOCX/XLSX/PPTX |
| Path reconstruction | ✅ | Full path from root to file |
| Permissions management | ✅ | Complete access control |
| Shared Drives support | ✅ | SupportsAllDrives enabled |
| OAuth2 via browser | ✅ | Local server callback |
| MCP HTTP Streamable | ✅ | 20 tools for AI agents |
| MCP OAuth2 server | ✅ | RFC 8414/9728/7591 + PKCE S256 |
| Cloud Run deployment | ✅ | Terraform-managed infrastructure |
| Custom domain | ✅ | drive.mcp.scm-platform.org |

## Detailed Documentation (.agent_docs/)

| File | Topic |
|------|-------|
| `.agent_docs/mcp-server.md` | MCP server architecture, all 20 tools, configuration |
| `.agent_docs/terraform.md` | Infrastructure, deployment workflow, GCP resources |
| `.agent_docs/authentication.md` | OAuth2 flow (CLI + MCP), context injection, endpoints |

## Related Documentation

- [Google Drive API v3](https://developers.google.com/drive/api/v3/reference)
- [Cobra Documentation](https://github.com/spf13/cobra)
- [Go OAuth2](https://pkg.go.dev/golang.org/x/oauth2)
- [Google API Go Client](https://pkg.go.dev/google.golang.org/api)

## Author

Sebastien MORAND - sebastien.morand@loreal.com
