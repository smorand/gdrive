// Package drive provides Google Drive API operations.
package drive

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/schollz/progressbar/v3"
	"google.golang.org/api/drive/v3"
)

const (
	// Google Drive special IDs
	DriveRootID   = "root"
	DriveSharedID = "shared"

	// Google Drive MIME types
	DriveFolderMimeType = "application/vnd.google-apps.folder"
	DriveDocMimeType    = "application/vnd.google-apps.document"
	DriveSheetMimeType  = "application/vnd.google-apps.spreadsheet"
	DriveSlideMimeType  = "application/vnd.google-apps.presentation"

	// Special folder names
	MyDriveName      = "My Drive"
	SharedWithMeName = "Shared with me"

	// File permissions
	downloadFilePerm = 0644
)

// MIMETypeMappings maps file type shortcuts to MIME types.
var MIMETypeMappings = map[string][]string{
	"image": {
		"image/jpeg", "image/jpg", "image/png", "image/gif",
		"image/bmp", "image/webp", "image/svg+xml", "image/tiff",
	},
	"audio": {
		"audio/mpeg", "audio/mp3", "audio/wav", "audio/ogg",
		"audio/aac", "audio/flac", "audio/m4a",
	},
	"video": {
		"video/mp4", "video/mpeg", "video/quicktime",
		"video/x-msvideo", "video/x-matroska", "video/webm", "video/avi",
	},
	"prez": {
		"application/vnd.ms-powerpoint",
		"application/vnd.openxmlformats-officedocument.presentationml.presentation",
		"application/vnd.google-apps.presentation",
	},
	"doc": {
		"application/msword",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.google-apps.document",
		"application/rtf",
	},
	"spreadsheet": {
		"application/vnd.ms-excel",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/vnd.google-apps.spreadsheet",
	},
	"txt":    {"text/plain", "text/csv", "text/html", "text/markdown"},
	"pdf":    {"application/pdf"},
	"folder": {"application/vnd.google-apps.folder"},
}

// ExportFormats maps Google Workspace MIME types to export formats.
var ExportFormats = map[string]map[string]string{
	"application/vnd.google-apps.document": {
		"pdf":  "application/pdf",
		"docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"txt":  "text/plain",
		"html": "text/html",
	},
	"application/vnd.google-apps.spreadsheet": {
		"xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"pdf":  "application/pdf",
		"csv":  "text/csv",
	},
	"application/vnd.google-apps.presentation": {
		"pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		"pdf":  "application/pdf",
	},
}

// Service wraps the Google Drive service.
type Service struct {
	API *drive.Service
}

// NewService creates a new DriveService.
func NewService(service *drive.Service) *Service {
	return &Service{API: service}
}

// ParseRemotePath parses a remote path into folder components.
func (ds *Service) ParseRemotePath(remotePath string) []string {
	path := strings.Trim(remotePath, "/")
	if path == "" {
		return []string{}
	}
	return strings.Split(path, "/")
}

// FindItemByName finds an item by name in a parent folder.
func (ds *Service) FindItemByName(name, parentID, mimeType string) (*drive.File, error) {
	query := fmt.Sprintf("name = '%s' and '%s' in parents and trashed = false", name, parentID)
	if mimeType != "" {
		query += fmt.Sprintf(" and mimeType = '%s'", mimeType)
	}

	fileList, err := ds.API.Files.List().Q(query).
		Fields("files(id, name, mimeType, modifiedTime, size)").Do()
	if err != nil {
		return nil, err
	}

	if len(fileList.Files) == 0 {
		return nil, nil
	}
	return fileList.Files[0], nil
}

// ResolvePath resolves a human-readable path to a folder ID.
func (ds *Service) ResolvePath(remotePath string, mustExist bool) (string, error) {
	parts := ds.ParseRemotePath(remotePath)
	if len(parts) == 0 {
		return DriveRootID, nil
	}

	currentID := DriveRootID
	for _, part := range parts {
		item, err := ds.FindItemByName(part, currentID, DriveFolderMimeType)
		if err != nil {
			return "", err
		}
		if item == nil {
			if mustExist {
				return "", fmt.Errorf("path not found: %s", remotePath)
			}
			return "", nil
		}
		currentID = item.Id
	}

	return currentID, nil
}

// CreateFolderPath creates a folder path (like mkdir -p).
func (ds *Service) CreateFolderPath(remotePath string) (string, error) {
	parts := ds.ParseRemotePath(remotePath)
	if len(parts) == 0 {
		return "root", nil
	}

	currentID := "root"
	for _, part := range parts {
		item, err := ds.FindItemByName(part, currentID, "application/vnd.google-apps.folder")
		if err != nil {
			return "", err
		}

		if item != nil {
			currentID = item.Id
		} else {
			// Create folder
			fileMetadata := &drive.File{
				Name:     part,
				MimeType: "application/vnd.google-apps.folder",
				Parents:  []string{currentID},
			}
			folder, err := ds.API.Files.Create(fileMetadata).Fields("id").Do()
			if err != nil {
				return "", err
			}
			currentID = folder.Id
			fmt.Printf("Created folder: %s\n", part)
		}
	}

	return currentID, nil
}

// FindFile finds a file by name in a parent folder.
func (ds *Service) FindFile(filename, parentID string) (*drive.File, error) {
	return ds.FindItemByName(filename, parentID, "")
}

// UploadFile uploads a file to Google Drive.
func (ds *Service) UploadFile(localPath, parentID string, showProgress bool) (string, error) {
	filename := filepath.Base(localPath)
	existingFile, err := ds.FindFile(filename, parentID)
	if err != nil {
		return "", err
	}

	file, err := os.Open(localPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return "", err
	}

	var reader io.Reader = file
	if showProgress {
		bar := progressbar.DefaultBytes(stat.Size(), fmt.Sprintf("Uploading %s", filename))
		reader = io.TeeReader(file, bar)
	}

	if existingFile != nil {
		// Update existing file
		if showProgress {
			fmt.Printf("Updating: %s\n", filename)
		}
		updatedFile, err := ds.API.Files.Update(existingFile.Id, &drive.File{}).Media(reader).Do()
		if err != nil {
			return "", err
		}
		return updatedFile.Id, nil
	}

	// Create new file
	if showProgress {
		fmt.Printf("Uploading: %s\n", filename)
	}
	fileMetadata := &drive.File{
		Name:    filename,
		Parents: []string{parentID},
	}
	createdFile, err := ds.API.Files.Create(fileMetadata).Media(reader).Fields("id").Do()
	if err != nil {
		return "", err
	}
	return createdFile.Id, nil
}

// DownloadFile downloads a file from Google Drive.
// For Google Workspace files, it exports them to standard formats.
func (ds *Service) DownloadFile(fileID, localPath string, preserveTimestamp, showProgress bool) error {
	// Get file metadata
	fileMetadata, err := ds.API.Files.Get(fileID).Fields("name, modifiedTime, size, mimeType").Do()
	if err != nil {
		return err
	}

	var resp *http.Response
	var exportFormat string

	// Check if it's a Google Workspace file
	if ds.IsGoogleWorkspaceFile(fileMetadata) {
		// Determine export format
		exportFormat = ds.GetDefaultExportFormat(fileMetadata.MimeType)
		if exportFormat == "" {
			return fmt.Errorf("cannot export file type: %s", fileMetadata.MimeType)
		}

		// Get export MIME type
		exportMimeType := ds.GetExportMimeType(fileMetadata.MimeType, exportFormat)
		if exportMimeType == "" {
			return fmt.Errorf("unknown export format: %s", exportFormat)
		}

		// Adjust filename extension
		localPath = ds.AdjustFilename(localPath, exportFormat)

		// Export file
		resp, err = ds.API.Files.Export(fileID, exportMimeType).Download()
		if err != nil {
			return err
		}
	} else {
		// Download regular file
		resp, err = ds.API.Files.Get(fileID).Download()
		if err != nil {
			return err
		}
	}
	defer resp.Body.Close()

	// Create local directory if needed
	dir := filepath.Dir(localPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Create local file
	localFile, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer localFile.Close()

	// Copy with progress
	if showProgress {
		// For exported files, the size from metadata is often inaccurate
		// Use -1 for indeterminate progress bar to avoid "current number exceeds max" error
		size := fileMetadata.Size
		if exportFormat != "" {
			// Exported files: use indeterminate progress (-1) since the export size differs from source size
			size = -1
		} else if size == 0 && resp.ContentLength > 0 {
			// Regular files with missing size: use ContentLength from response
			size = resp.ContentLength
		}
		bar := progressbar.DefaultBytes(size, fmt.Sprintf("Downloading %s", fileMetadata.Name))
		_, err = io.Copy(io.MultiWriter(localFile, bar), resp.Body)
	} else {
		_, err = io.Copy(localFile, resp.Body)
	}
	if err != nil {
		return err
	}

	// Preserve timestamp
	if preserveTimestamp && fileMetadata.ModifiedTime != "" {
		modTime, err := time.Parse(time.RFC3339, fileMetadata.ModifiedTime)
		if err == nil {
			os.Chtimes(localPath, modTime, modTime)
		}
	}

	return nil
}

// ListFolder lists all items in a folder.
func (ds *Service) ListFolder(folderID string) ([]*drive.File, error) {
	query := fmt.Sprintf("'%s' in parents and trashed = false", folderID)
	fileList, err := ds.API.Files.List().Q(query).
		Fields("files(id, name, mimeType, modifiedTime, size)").
		PageSize(1000).Do()
	if err != nil {
		return nil, err
	}
	return fileList.Files, nil
}

// IsFolder checks if an item is a folder.
func (ds *Service) IsFolder(item *drive.File) bool {
	return item.MimeType == "application/vnd.google-apps.folder"
}

// IsGoogleWorkspaceFile checks if an item is a Google Workspace file.
func (ds *Service) IsGoogleWorkspaceFile(item *drive.File) bool {
	workspaceTypes := []string{
		"application/vnd.google-apps.document",
		"application/vnd.google-apps.spreadsheet",
		"application/vnd.google-apps.presentation",
		"application/vnd.google-apps.form",
		"application/vnd.google-apps.drawing",
		"application/vnd.google-apps.map",
		"application/vnd.google-apps.site",
	}

	for _, wsType := range workspaceTypes {
		if item.MimeType == wsType {
			return true
		}
	}
	return false
}

// ExpandFileTypes expands file type shortcuts to MIME types.
func (ds *Service) ExpandFileTypes(fileTypes []string) []string {
	var mimeTypes []string
	seen := make(map[string]bool)

	for _, fileType := range fileTypes {
		// If it's already a MIME type (contains '/'), use it directly
		if strings.Contains(fileType, "/") {
			if !seen[fileType] {
				mimeTypes = append(mimeTypes, fileType)
				seen[fileType] = true
			}
		} else if types, ok := MIMETypeMappings[strings.ToLower(fileType)]; ok {
			// Look up in mappings
			for _, mimeType := range types {
				if !seen[mimeType] {
					mimeTypes = append(mimeTypes, mimeType)
					seen[mimeType] = true
				}
			}
		} else {
			fmt.Printf("Warning: Unknown file type '%s', ignoring\n", fileType)
		}
	}

	return mimeTypes
}

// SearchFiles searches for files and folders on Google Drive.
func (ds *Service) SearchFiles(query string, fileTypes []string, maxResults int64) ([]*drive.File, error) {
	searchQuery := fmt.Sprintf("name contains '%s' and trashed = false", query)

	// Add MIME type filters if specified
	if len(fileTypes) > 0 {
		mimeTypes := ds.ExpandFileTypes(fileTypes)
		if len(mimeTypes) > 0 {
			var mimeConditions []string
			for _, mime := range mimeTypes {
				mimeConditions = append(mimeConditions, fmt.Sprintf("mimeType = '%s'", mime))
			}
			searchQuery += fmt.Sprintf(" and (%s)", strings.Join(mimeConditions, " or "))
		}
	}

	fileList, err := ds.API.Files.List().Q(searchQuery).
		Fields("files(id, name, mimeType, modifiedTime, size)").
		PageSize(maxResults).Do()
	if err != nil {
		return nil, err
	}

	return fileList.Files, nil
}

// DeleteFile deletes a file or folder from Google Drive.
func (ds *Service) DeleteFile(fileID string) error {
	return ds.API.Files.Delete(fileID).Do()
}

// RenameFile renames a file or folder.
func (ds *Service) RenameFile(fileID, newName string) (*drive.File, error) {
	fileMetadata := &drive.File{Name: newName}
	return ds.API.Files.Update(fileID, fileMetadata).
		Fields("id, name, webViewLink").Do()
}

// MoveFile moves a file to a different folder.
func (ds *Service) MoveFile(fileID, targetFolderID string) (*drive.File, error) {
	// Get current parents
	file, err := ds.API.Files.Get(fileID).Fields("parents").Do()
	if err != nil {
		return nil, err
	}

	// Build previous parents string
	previousParents := strings.Join(file.Parents, ",")

	// Move file
	return ds.API.Files.Update(fileID, &drive.File{}).
		AddParents(targetFolderID).
		RemoveParents(previousParents).
		Fields("id, name, parents").Do()
}

// CopyOptions holds options for copying a file.
type CopyOptions struct {
	NewName        string
	ParentFolderID string
}

// CopyFile copies a file in Google Drive.
func (ds *Service) CopyFile(fileID string, opts CopyOptions) (*drive.File, error) {
	body := &drive.File{}
	if opts.NewName != "" {
		body.Name = opts.NewName
	}
	if opts.ParentFolderID != "" {
		body.Parents = []string{opts.ParentFolderID}
	}

	return ds.API.Files.Copy(fileID, body).
		Fields("id, name, webViewLink").Do()
}

// PathComponent represents a component in a file path.
type PathComponent struct {
	ID       string
	Name     string
	MimeType string
}

// GetFilePath reconstructs the full path of a file by traversing parent folders.
func (ds *Service) GetFilePath(fileID string) ([]PathComponent, error) {
	var path []PathComponent
	currentID := fileID

	for currentID != "" {
		file, err := ds.API.Files.Get(currentID).
			Fields("id, name, parents, mimeType, sharedWithMeTime").
			SupportsAllDrives(true).Do()
		if err != nil {
			break
		}

		// Add to beginning of path
		path = append([]PathComponent{{
			ID:       file.Id,
			Name:     file.Name,
			MimeType: file.MimeType,
		}}, path...)

		// Move to parent
		if len(file.Parents) > 0 {
			currentID = file.Parents[0]
		} else {
			// Root reached
			if file.SharedWithMeTime != "" {
				// Shared file
				path = append([]PathComponent{{
					ID:       "shared",
					Name:     "Shared with me",
					MimeType: "special",
				}}, path...)
			} else {
				// My Drive
				path = append([]PathComponent{{
					ID:       "root",
					Name:     "My Drive",
					MimeType: "special",
				}}, path...)
			}
			break
		}
	}

	return path, nil
}

// FileInfo contains detailed file information.
type FileInfo struct {
	ID           string
	Name         string
	MimeType     string
	Size         int64
	CreatedTime  string
	ModifiedTime string
	WebViewLink  string
	Owners       []*drive.User
	Path         []PathComponent
}

// GetFileInfo retrieves detailed information about a file.
func (ds *Service) GetFileInfo(fileID string) (*FileInfo, error) {
	file, err := ds.API.Files.Get(fileID).
		Fields("id, name, mimeType, size, createdTime, modifiedTime, webViewLink, owners").
		Do()
	if err != nil {
		return nil, err
	}

	path, _ := ds.GetFilePath(fileID)

	return &FileInfo{
		ID:           file.Id,
		Name:         file.Name,
		MimeType:     file.MimeType,
		Size:         file.Size,
		CreatedTime:  file.CreatedTime,
		ModifiedTime: file.ModifiedTime,
		WebViewLink:  file.WebViewLink,
		Owners:       file.Owners,
		Path:         path,
	}, nil
}

// ShareOptions holds options for sharing a file.
type ShareOptions struct {
	Email   string
	Role    string
	Notify  bool
	Message string
}

// ShareFile shares a file with a user.
func (ds *Service) ShareFile(fileID string, opts ShareOptions) error {
	permission := &drive.Permission{
		Type:         "user",
		Role:         opts.Role,
		EmailAddress: opts.Email,
	}

	createCall := ds.API.Permissions.Create(fileID, permission).
		Fields("id").
		SendNotificationEmail(opts.Notify).
		SupportsAllDrives(true)

	if opts.Message != "" {
		createCall = createCall.EmailMessage(opts.Message)
	}

	_, err := createCall.Do()
	return err
}

// ShareWithAnyone shares a file with anyone who has the link.
func (ds *Service) ShareWithAnyone(fileID, role string) error {
	permission := &drive.Permission{
		Type: "anyone",
		Role: role,
	}

	_, err := ds.API.Permissions.Create(fileID, permission).
		Fields("id").
		SupportsAllDrives(true).Do()
	return err
}

// ListPermissions lists all permissions for a file.
func (ds *Service) ListPermissions(fileID string) ([]*drive.Permission, error) {
	perms, err := ds.API.Permissions.List(fileID).
		Fields("permissions(id, type, role, emailAddress, displayName, domain)").
		SupportsAllDrives(true).Do()
	if err != nil {
		return nil, err
	}

	return perms.Permissions, nil
}

// RemovePermission removes a specific permission from a file.
func (ds *Service) RemovePermission(fileID, permissionID string) error {
	return ds.API.Permissions.Delete(fileID, permissionID).
		SupportsAllDrives(true).Do()
}

// RemovePublicAccess removes public access (anyone with the link) from a file.
func (ds *Service) RemovePublicAccess(fileID string) error {
	perms, err := ds.ListPermissions(fileID)
	if err != nil {
		return err
	}

	for _, perm := range perms {
		if perm.Type == "anyone" {
			if err := ds.RemovePermission(fileID, perm.Id); err != nil {
				return err
			}
		}
	}

	return nil
}

// GetExportMimeType returns the MIME type for exporting a Google Workspace file.
func (ds *Service) GetExportMimeType(workspaceMimeType, format string) string {
	if formats, ok := ExportFormats[workspaceMimeType]; ok {
		if mimeType, ok := formats[format]; ok {
			return mimeType
		}
	}
	return ""
}

// GetDefaultExportFormat returns the default export format for a Google Workspace file.
func (ds *Service) GetDefaultExportFormat(mimeType string) string {
	switch mimeType {
	case "application/vnd.google-apps.document":
		return "pdf"
	case "application/vnd.google-apps.spreadsheet":
		return "xlsx"
	case "application/vnd.google-apps.presentation":
		return "pptx"
	default:
		return ""
	}
}

// AdjustFilename adjusts the filename extension based on export format.
func (ds *Service) AdjustFilename(localPath, exportFormat string) string {
	if exportFormat == "" {
		return localPath
	}

	// Remove existing extension
	ext := filepath.Ext(localPath)
	baseName := strings.TrimSuffix(localPath, ext)

	// Add new extension
	return baseName + "." + exportFormat
}
