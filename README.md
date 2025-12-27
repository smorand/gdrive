# gdrive

A command-line tool for syncing files and folders with Google Drive, built in Go for performance and portability.

## Features

- 📥 **File Download**: Download files from Google Drive with overwrite protection
- 📤 **File Upload**: Upload files to Google Drive (creates new versions for existing files)
- 🗑️ **File Management**: Delete, rename, move, and copy files
- 📋 **File Info**: Display detailed file information including full path
- 📁 **Folder Operations**: Create, upload, download folders recursively
- ⚡ **Parallel Downloads**: Concurrent file downloads (configurable 1-20, default 5)
- 🔍 **Search**: Find files and folders with MIME type filtering
- 📊 **Progress Tracking**: Real-time progress bars for uploads and downloads
- 🆔 **ID Support**: Use Google Drive IDs directly with `--id` flag
- ⏱️ **Timestamp Preservation**: Maintains original modification times
- 🔐 **Permissions Management**: Share files, manage permissions, control access
- 📦 **Google Workspace Export**: Automatic export to standard formats (PDF, DOCX, XLSX, PPTX)
- 📜 **Activity Tracking**: View recent changes and file revision history

## Installation

### Prerequisites

- Go 1.21 or higher
- Google Cloud project with Drive API enabled
- OAuth2 credentials (`credentials.json`)

### Build

```bash
./build.sh
```

This creates a `gdrive` binary in the project root.

To install globally:
```bash
sudo mv gdrive /usr/local/bin/
```

## Authentication

1. Create a Google Cloud project and enable the Drive API
2. Create OAuth2 credentials (Desktop application)
3. Download `credentials.json` and place it in:
   - Current directory, OR
   - `~/.gdrive/credentials.json`

On first run, you'll authenticate via browser and credentials will be saved to `~/.gdrive/token.json`.

## Configuration

### Config Directory and Credentials

You can customize where `gdrive` looks for configuration files using three methods (in priority order):

1. **Command-line flags** (highest priority):
   ```bash
   gdrive --config-dir /custom/path --credentials /path/to/credentials.json file list Documents
   ```

2. **Environment variables**:
   ```bash
   export GDRIVE_CONFIG_DIR="/custom/path"
   export GDRIVE_CREDENTIALS_PATH="/path/to/credentials.json"
   gdrive file list Documents
   ```

3. **Default values** (lowest priority):
   - Config directory: `$HOME/.gdrive`
   - Credentials: `./credentials.json` or `$HOME/.gdrive/credentials.json`

**Global Flags:**
- `--config-dir` - Directory for storing token.json (env: `GDRIVE_CONFIG_DIR`)
- `--credentials` - Path to credentials.json file (env: `GDRIVE_CREDENTIALS_PATH`)

These flags work with all commands and allow you to manage multiple Google accounts or use custom paths.

## Usage

### File Operations

**Download a file:**
```bash
gdrive file download Parameters/file.txt
gdrive file download Parameters/file.txt ./downloads
gdrive file download Parameters/file.txt ./downloads --overwrite
gdrive file download 1a2b3c4d5e --id
```

**Upload a file:**
```bash
gdrive file upload ./myfile.txt Parameters/bin
gdrive file upload /path/to/file.pdf Documents
gdrive file upload ./myfile.txt 1a2b3c4d5e --id
```

**Delete a file:**
```bash
gdrive file delete Parameters/file.txt
gdrive file delete 1a2b3c4d5e --id
```

**Rename a file:**
```bash
gdrive file rename Parameters/old.txt new.txt
gdrive file rename 1a2b3c4d5e new_name.txt --id
```

**Move a file:**
```bash
gdrive file move Parameters/file.txt Documents
gdrive file move 1a2b3c4d5e 1xyz789 --id
```

**Copy a file:**
```bash
gdrive file copy Parameters/file.txt
gdrive file copy Parameters/file.txt "Copy of file.txt"
gdrive file copy Parameters/file.txt --parent Documents
gdrive file copy 1a2b3c4d5e --id
```

**Get file info:**
```bash
gdrive file info Parameters/file.txt
gdrive file info 1a2b3c4d5e --id
```

**Share a file:**
```bash
gdrive file share Parameters/file.txt user@example.com
gdrive file share Parameters/file.txt user@example.com --role writer
gdrive file share 1a2b3c4d5e user@example.com --id --no-notify
gdrive file share Parameters/file.txt user@example.com --message "Check this out!"
```

**Share file publicly:**
```bash
gdrive file share-public Parameters/file.txt
gdrive file share-public Parameters/file.txt --role writer
gdrive file share-public 1a2b3c4d5e --id
```

**List file permissions:**
```bash
gdrive file permissions Parameters/file.txt
gdrive file permissions 1a2b3c4d5e --id
```

**Remove a permission:**
```bash
gdrive file remove-permission Parameters/file.txt 12345678
gdrive file remove-permission 1a2b3c4d5e 12345678 --id
```

**Remove public access:**
```bash
gdrive file remove-public Parameters/file.txt
gdrive file remove-public 1a2b3c4d5e --id
```

### Folder Operations

**Create a folder:**
```bash
gdrive folder create Parameters/bin
gdrive folder create Documents/Projects/2024
```

**Upload a folder:**
```bash
gdrive folder upload ./my_project Parameters/Projects
gdrive folder upload /path/to/folder Documents/Backup
gdrive folder upload ./my_project 1a2b3c4d5e --id
```

**Download a folder:**
```bash
gdrive folder download Parameters/bin ./downloads
gdrive folder download Documents/Projects ./backup --overwrite
gdrive folder download 1a2b3c4d5e ./downloads --id
gdrive folder download Documents ./backup --parallel 10        # Use 10 concurrent downloads
gdrive folder download Documents ./backup --new-only           # Only download new/newer files
gdrive folder download Documents ./backup --new-only --overwrite  # Auto-update newer files
```

**Flags:**
- `--parallel, -p`: Number of concurrent downloads (default: 5, range: 1-20)
- `--new-only`: Skip files that exist locally unless Drive version is newer
  - Without `--overwrite`: Asks before downloading newer files
  - With `--overwrite`: Automatically downloads newer files

**List folder contents:**
```bash
gdrive folder list Parameters/bin
gdrive folder list Documents
gdrive folder list 1a2b3c4d5e --id
```

### Activity & Revision History

**View recent changes:**
```bash
gdrive activity changes
gdrive activity changes --max 20
```

**View deleted files:**
```bash
gdrive activity deleted                    # Last 7 days (default)
gdrive activity deleted --days 14          # Last 14 days
gdrive activity deleted --days 30 --max 50 # Last 30 days, max 50 results
```

**View comprehensive activity history (includes permanent deletions):**
```bash
gdrive activity history                    # Last 7 days (default)
gdrive activity history --days 14          # Last 14 days
gdrive activity history --days 30 --max 200 # Last 30 days
```

**View file revision history:**
```bash
gdrive activity revisions Parameters/file.txt
gdrive activity revisions 1a2b3c4d5e --id
```

### Search

**Basic search:**
```bash
gdrive search report
gdrive search "budget 2024" --max 20
```

**Search with file type filters:**
```bash
gdrive search Passeport --type image,pdf
gdrive search Passeport --type pdf,image/jpeg
gdrive search "My Project" --type folder
gdrive search contract --type doc -m 10
```

**Available type shortcuts:**
- `image`: JPEG, PNG, GIF, BMP, WebP, SVG, TIFF
- `audio`: MP3, WAV, OGG, AAC, FLAC, M4A
- `video`: MP4, MPEG, QuickTime, AVI, MKV, WebM
- `prez`: PowerPoint, Google Slides
- `doc`: Word, Google Docs, RTF
- `spreadsheet`: Excel, Google Sheets
- `txt`: Plain text, CSV, HTML, Markdown
- `pdf`: PDF files
- `folder`: Folders only

You can also use explicit MIME types like `image/jpeg` or `application/pdf`.

## Command Reference

### File Commands

- `gdrive file download REMOTE_FILE [LOCAL_FOLDER]` - Download a file
  - `--overwrite` - Overwrite without asking
  - `--id` - Treat REMOTE_FILE as a Drive file ID

- `gdrive file upload LOCAL_FILE REMOTE_FOLDER` - Upload a file
  - `--id` - Treat REMOTE_FOLDER as a Drive folder ID

- `gdrive file delete FILE` - Delete a file
  - `--id` - Treat FILE as a Drive file ID

- `gdrive file rename FILE NEW_NAME` - Rename a file
  - `--id` - Treat FILE as a Drive file ID

- `gdrive file move FILE TARGET_FOLDER` - Move a file to another folder
  - `--id` - Treat FILE and TARGET_FOLDER as Drive IDs

- `gdrive file copy FILE [NEW_NAME]` - Copy a file
  - `--id` - Treat FILE as a Drive file ID
  - `--parent` - Parent folder path or ID for the copy

- `gdrive file info FILE` - Display detailed file information
  - `--id` - Treat FILE as a Drive file ID

- `gdrive file share FILE EMAIL` - Share a file with a user
  - `--id` - Treat FILE as a Drive file ID
  - `--role` - Permission role: reader (default), writer, commenter
  - `--no-notify` - Don't send notification email
  - `--message` - Custom message for notification email

- `gdrive file share-public FILE` - Share with anyone who has the link
  - `--id` - Treat FILE as a Drive file ID
  - `--role` - Permission role: reader (default), writer, commenter

- `gdrive file permissions FILE` - List all permissions for a file
  - `--id` - Treat FILE as a Drive file ID

- `gdrive file remove-permission FILE PERMISSION_ID` - Remove a specific permission
  - `--id` - Treat FILE as a Drive file ID

- `gdrive file remove-public FILE` - Remove public access from a file
  - `--id` - Treat FILE as a Drive file ID

### Folder Commands

- `gdrive folder create REMOTE_FOLDER` - Create folder path (mkdir -p style)

- `gdrive folder upload LOCAL_SRC REMOTE_FOLDER` - Upload folder recursively
  - `--id` - Treat REMOTE_FOLDER as a Drive folder ID

- `gdrive folder download REMOTE_FOLDER LOCAL_FOLDER` - Download folder recursively
  - `--overwrite` - Overwrite without asking
  - `--id` - Treat REMOTE_FOLDER as a Drive folder ID
  - `--parallel, -p` - Number of parallel downloads (1-20, default: 5)
  - `--new-only` - Only download new or newer files from Drive

- `gdrive folder list REMOTE_FOLDER` - List folder contents
  - `--id` - Treat REMOTE_FOLDER as a Drive folder ID

### Activity Commands

- `gdrive activity changes` - List recent changes to files
  - `--max, -m` - Maximum number of changes to show (default: 50)

- `gdrive activity deleted` - List recently deleted files (in trash)
  - `--days` - Number of days back to search (default: 7)
  - `--max, -m` - Maximum number of deleted files to show (default: 100)

- `gdrive activity history` - List comprehensive activity history (Drive Activity API)
  - Includes permanent deletions, edits, moves, permission changes, and more
  - `--days` - Number of days back to show (default: 7)
  - `--max, -m` - Maximum number of activities to show (default: 100)

- `gdrive activity revisions FILE` - List revision history for a file
  - `--id` - Treat FILE as a Drive file ID

### Search Command

- `gdrive search QUERY` - Search for files and folders
  - `--max, -m` - Maximum results (default: 50)
  - `--type, -t` - File type filter (comma-separated)

## Architecture

```
gdrive-go/
├── src/
│   ├── main.go          # Main entry point
│   ├── cli.go           # CLI commands implementation
│   ├── auth.go          # OAuth2 authentication
│   ├── drive_service.go # Drive API operations
│   └── go.mod           # Go dependencies
├── build.sh             # Build script
└── README.md
```

## Key Features

✅ Intuitive CLI interface with Cobra
✅ Credential storage in `~/.gdrive/`
✅ Smart path resolution (e.g., `Documents/Projects/2024`)
✅ Automatic file versioning via Google Drive
✅ Real-time progress tracking
✅ Comprehensive MIME type support
✅ Direct ID support with `--id` flag
✅ Google Workspace file export (automatic conversion to PDF/DOCX/XLSX/PPTX)
✅ Overwrite protection with confirmations
✅ Timestamp preservation on downloads
✅ Complete file management (delete, rename, move, copy)
✅ File information with full path reconstruction
✅ Permissions management (share, list, remove)
✅ Public sharing control

## Google Workspace Files

Google Workspace files (Docs, Sheets, Slides) are automatically exported to standard formats:
- **Google Docs** → PDF (default)
- **Google Sheets** → XLSX (default)
- **Google Slides** → PPTX (default)

Other Google Workspace files (Forms, Drawings, Maps, Sites) cannot be downloaded and are automatically skipped with a warning message.

The export happens transparently during `file download` and `folder download` operations, with proper file extension adjustment.

## Error Handling

- Authentication errors: Check `credentials.json` exists and is valid
- Path not found: Verify folder exists or create it first with `folder create`
- Upload failures: Ensure target folder exists before uploading files

## Performance

Built for speed and efficiency:
- **Parallel downloads**: Download multiple files concurrently (configurable 1-20, default: 5)
- **Compiled binary**: No interpreter overhead, instant startup
- **Native concurrency**: Leverages Go's goroutines for efficient resource usage
- **Optimized memory**: Efficient buffer management for large file operations

**Example**: Downloading a folder with 100 files takes ~10-20 seconds with `--parallel 10` depending on file sizes and network speed.

## License

MIT License

## Author

Sebastien MORAND
