package main

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
	"google.golang.org/api/driveactivity/v2"
)

const (
	// Google Drive special IDs
	driveRootID = "root"
	driveSharedID = "shared"

	// Google Drive MIME types
	driveFolderMimeType = "application/vnd.google-apps.folder"
	driveDocMimeType    = "application/vnd.google-apps.document"
	driveSheetMimeType  = "application/vnd.google-apps.spreadsheet"
	driveSlideMimeType  = "application/vnd.google-apps.presentation"

	// Special folder names
	myDriveName     = "My Drive"
	sharedWithMeName = "Shared with me"

	// File permissions
	downloadFilePerm = 0644
)

// MIME type mappings for file type shortcuts
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

// DriveService wraps the Google Drive service
type DriveService struct {
	Service *drive.Service
}

// NewDriveService creates a new DriveService
func NewDriveService(service *drive.Service) *DriveService {
	return &DriveService{Service: service}
}

// ParseRemotePath parses a remote path into folder components
func (ds *DriveService) ParseRemotePath(remotePath string) []string {
	path := strings.Trim(remotePath, "/")
	if path == "" {
		return []string{}
	}
	return strings.Split(path, "/")
}

// FindItemByName finds an item by name in a parent folder
func (ds *DriveService) FindItemByName(name, parentID, mimeType string) (*drive.File, error) {
	query := fmt.Sprintf("name = '%s' and '%s' in parents and trashed = false", name, parentID)
	if mimeType != "" {
		query += fmt.Sprintf(" and mimeType = '%s'", mimeType)
	}

	fileList, err := ds.Service.Files.List().Q(query).
		Fields("files(id, name, mimeType, modifiedTime, size)").Do()
	if err != nil {
		return nil, err
	}

	if len(fileList.Files) == 0 {
		return nil, nil
	}
	return fileList.Files[0], nil
}

// ResolvePath resolves a human-readable path to a folder ID
func (ds *DriveService) ResolvePath(remotePath string, mustExist bool) (string, error) {
	parts := ds.ParseRemotePath(remotePath)
	if len(parts) == 0 {
		return driveRootID, nil
	}

	currentID := driveRootID
	for _, part := range parts {
		item, err := ds.FindItemByName(part, currentID, driveFolderMimeType)
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

// CreateFolderPath creates a folder path (like mkdir -p)
func (ds *DriveService) CreateFolderPath(remotePath string) (string, error) {
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
			folder, err := ds.Service.Files.Create(fileMetadata).Fields("id").Do()
			if err != nil {
				return "", err
			}
			currentID = folder.Id
			fmt.Printf("Created folder: %s\n", part)
		}
	}

	return currentID, nil
}

// FindFile finds a file by name in a parent folder
func (ds *DriveService) FindFile(filename, parentID string) (*drive.File, error) {
	return ds.FindItemByName(filename, parentID, "")
}

// UploadFile uploads a file to Google Drive
func (ds *DriveService) UploadFile(localPath, parentID string, showProgress bool) (string, error) {
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
		updatedFile, err := ds.Service.Files.Update(existingFile.Id, &drive.File{}).Media(reader).Do()
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
	createdFile, err := ds.Service.Files.Create(fileMetadata).Media(reader).Fields("id").Do()
	if err != nil {
		return "", err
	}
	return createdFile.Id, nil
}

// DownloadFile downloads a file from Google Drive
// For Google Workspace files, it exports them to standard formats
func (ds *DriveService) DownloadFile(fileID, localPath string, preserveTimestamp, showProgress bool) error {
	// Get file metadata
	fileMetadata, err := ds.Service.Files.Get(fileID).Fields("name, modifiedTime, size, mimeType").Do()
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
		resp, err = ds.Service.Files.Export(fileID, exportMimeType).Download()
		if err != nil {
			return err
		}
	} else {
		// Download regular file
		resp, err = ds.Service.Files.Get(fileID).Download()
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

// ListFolder lists all items in a folder
func (ds *DriveService) ListFolder(folderID string) ([]*drive.File, error) {
	query := fmt.Sprintf("'%s' in parents and trashed = false", folderID)
	fileList, err := ds.Service.Files.List().Q(query).
		Fields("files(id, name, mimeType, modifiedTime, size)").
		PageSize(1000).Do()
	if err != nil {
		return nil, err
	}
	return fileList.Files, nil
}

// IsFolder checks if an item is a folder
func (ds *DriveService) IsFolder(item *drive.File) bool {
	return item.MimeType == "application/vnd.google-apps.folder"
}

// IsGoogleWorkspaceFile checks if an item is a Google Workspace file
func (ds *DriveService) IsGoogleWorkspaceFile(item *drive.File) bool {
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

// ExpandFileTypes expands file type shortcuts to MIME types
func (ds *DriveService) ExpandFileTypes(fileTypes []string) []string {
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

// SearchFiles searches for files and folders on Google Drive
func (ds *DriveService) SearchFiles(query string, fileTypes []string, maxResults int64) ([]*drive.File, error) {
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

	fileList, err := ds.Service.Files.List().Q(searchQuery).
		Fields("files(id, name, mimeType, modifiedTime, size)").
		PageSize(maxResults).Do()
	if err != nil {
		return nil, err
	}

	return fileList.Files, nil
}

// DeleteFile deletes a file or folder from Google Drive
func (ds *DriveService) DeleteFile(fileID string) error {
	return ds.Service.Files.Delete(fileID).Do()
}

// RenameFile renames a file or folder
func (ds *DriveService) RenameFile(fileID, newName string) (*drive.File, error) {
	fileMetadata := &drive.File{Name: newName}
	return ds.Service.Files.Update(fileID, fileMetadata).
		Fields("id, name, webViewLink").Do()
}

// MoveFile moves a file to a different folder
func (ds *DriveService) MoveFile(fileID, targetFolderID string) (*drive.File, error) {
	// Get current parents
	file, err := ds.Service.Files.Get(fileID).Fields("parents").Do()
	if err != nil {
		return nil, err
	}

	// Build previous parents string
	previousParents := strings.Join(file.Parents, ",")

	// Move file
	return ds.Service.Files.Update(fileID, &drive.File{}).
		AddParents(targetFolderID).
		RemoveParents(previousParents).
		Fields("id, name, parents").Do()
}

// CopyOptions holds options for copying a file
type CopyOptions struct {
	NewName        string
	ParentFolderID string
}

// CopyFile copies a file in Google Drive
func (ds *DriveService) CopyFile(fileID string, opts CopyOptions) (*drive.File, error) {
	body := &drive.File{}
	if opts.NewName != "" {
		body.Name = opts.NewName
	}
	if opts.ParentFolderID != "" {
		body.Parents = []string{opts.ParentFolderID}
	}

	return ds.Service.Files.Copy(fileID, body).
		Fields("id, name, webViewLink").Do()
}

// PathComponent represents a component in a file path
type PathComponent struct {
	ID       string
	Name     string
	MimeType string
}

// GetFilePath reconstructs the full path of a file by traversing parent folders
func (ds *DriveService) GetFilePath(fileID string) ([]PathComponent, error) {
	var path []PathComponent
	currentID := fileID

	for currentID != "" {
		file, err := ds.Service.Files.Get(currentID).
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

// FileInfo contains detailed file information
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

// GetFileInfo retrieves detailed information about a file
func (ds *DriveService) GetFileInfo(fileID string) (*FileInfo, error) {
	file, err := ds.Service.Files.Get(fileID).
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

// ShareOptions holds options for sharing a file
type ShareOptions struct {
	Email   string
	Role    string
	Notify  bool
	Message string
}

// ShareFile shares a file with a user
func (ds *DriveService) ShareFile(fileID string, opts ShareOptions) error {
	permission := &drive.Permission{
		Type:         "user",
		Role:         opts.Role,
		EmailAddress: opts.Email,
	}

	createCall := ds.Service.Permissions.Create(fileID, permission).
		Fields("id").
		SendNotificationEmail(opts.Notify).
		SupportsAllDrives(true)

	if opts.Message != "" {
		createCall = createCall.EmailMessage(opts.Message)
	}

	_, err := createCall.Do()
	return err
}

// ShareWithAnyone shares a file with anyone who has the link
func (ds *DriveService) ShareWithAnyone(fileID, role string) error {
	permission := &drive.Permission{
		Type: "anyone",
		Role: role,
	}

	_, err := ds.Service.Permissions.Create(fileID, permission).
		Fields("id").
		SupportsAllDrives(true).Do()
	return err
}

// ListPermissions lists all permissions for a file
func (ds *DriveService) ListPermissions(fileID string) ([]*drive.Permission, error) {
	perms, err := ds.Service.Permissions.List(fileID).
		Fields("permissions(id, type, role, emailAddress, displayName, domain)").
		SupportsAllDrives(true).Do()
	if err != nil {
		return nil, err
	}

	return perms.Permissions, nil
}

// RemovePermission removes a specific permission from a file
func (ds *DriveService) RemovePermission(fileID, permissionID string) error {
	return ds.Service.Permissions.Delete(fileID, permissionID).
		SupportsAllDrives(true).Do()
}

// RemovePublicAccess removes public access (anyone with the link) from a file
func (ds *DriveService) RemovePublicAccess(fileID string) error {
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

// ExportFormats maps Google Workspace MIME types to export formats
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

// GetExportMimeType returns the MIME type for exporting a Google Workspace file
func (ds *DriveService) GetExportMimeType(workspaceMimeType, format string) string {
	if formats, ok := ExportFormats[workspaceMimeType]; ok {
		if mimeType, ok := formats[format]; ok {
			return mimeType
		}
	}
	return ""
}

// GetDefaultExportFormat returns the default export format for a Google Workspace file
func (ds *DriveService) GetDefaultExportFormat(mimeType string) string {
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

// AdjustFilename adjusts the filename extension based on export format
func (ds *DriveService) AdjustFilename(localPath, exportFormat string) string {
	if exportFormat == "" {
		return localPath
	}

	// Remove existing extension
	ext := filepath.Ext(localPath)
	baseName := strings.TrimSuffix(localPath, ext)

	// Add new extension
	return baseName + "." + exportFormat
}

// ChangeInfo represents simplified change information
type ChangeInfo struct {
	FileID       string
	FileName     string
	ChangeTime   time.Time
	ChangeType   string
	Removed      bool
	MimeType     string
	ModifiedBy   string
}

// ListChanges lists recent changes to files in the Drive
func (ds *DriveService) ListChanges(pageSize int64) ([]*ChangeInfo, error) {
	// Get the start page token
	startToken, err := ds.Service.Changes.GetStartPageToken().Do()
	if err != nil {
		return nil, fmt.Errorf("unable to get start page token: %v", err)
	}

	// List changes from the start token
	changeList, err := ds.Service.Changes.List(startToken.StartPageToken).
		PageSize(pageSize).
		Fields("changes(file(id, name, mimeType, modifiedTime, modifiedByMeTime, lastModifyingUser), fileId, removed, time)").
		Do()
	if err != nil {
		return nil, fmt.Errorf("unable to list changes: %v", err)
	}

	var changes []*ChangeInfo
	for _, change := range changeList.Changes {
		changeInfo := &ChangeInfo{
			FileID:     change.FileId,
			Removed:    change.Removed,
			ChangeTime: time.Now(), // Default to now if not available
		}

		if change.Time != "" {
			if t, err := time.Parse(time.RFC3339, change.Time); err == nil {
				changeInfo.ChangeTime = t
			}
		}

		if change.File != nil {
			changeInfo.FileName = change.File.Name
			changeInfo.MimeType = change.File.MimeType

			if change.File.LastModifyingUser != nil {
				changeInfo.ModifiedBy = change.File.LastModifyingUser.DisplayName
				if changeInfo.ModifiedBy == "" {
					changeInfo.ModifiedBy = change.File.LastModifyingUser.EmailAddress
				}
			}

			// Determine change type
			if change.Removed {
				changeInfo.ChangeType = "Removed"
			} else if change.File.ModifiedTime != "" {
				changeInfo.ChangeType = "Modified"
			} else {
				changeInfo.ChangeType = "Added"
			}
		}

		changes = append(changes, changeInfo)
	}

	return changes, nil
}

// ListTrashedFiles lists files in the trash, optionally filtered by time
func (ds *DriveService) ListTrashedFiles(daysBack int, maxResults int64) ([]*drive.File, error) {
	// Build query for trashed files
	query := "trashed = true"

	// If days back is specified, add time filter
	if daysBack > 0 {
		cutoffTime := time.Now().AddDate(0, 0, -daysBack).Format(time.RFC3339)
		query = fmt.Sprintf("trashed = true and trashedTime >= '%s'", cutoffTime)
	}

	fileList, err := ds.Service.Files.List().
		Q(query).
		PageSize(maxResults).
		Fields("files(id, name, mimeType, trashedTime, trashingUser, size, parents)").
		OrderBy("trashedTime desc").
		Do()

	if err != nil {
		return nil, fmt.Errorf("unable to list trashed files: %v", err)
	}

	return fileList.Files, nil
}

// RevisionInfo represents file revision information
type RevisionInfo struct {
	ID              string
	ModifiedTime    time.Time
	Size            int64
	MimeType        string
	ModifiedBy      string
	KeepForever     bool
	Published       bool
}

// ListRevisions lists all revisions for a specific file
func (ds *DriveService) ListRevisions(fileID string) ([]*RevisionInfo, error) {
	revList, err := ds.Service.Revisions.List(fileID).
		Fields("revisions(id, modifiedTime, size, mimeType, lastModifyingUser, keepForever, published)").
		Do()
	if err != nil {
		return nil, fmt.Errorf("unable to list revisions: %v", err)
	}

	var revisions []*RevisionInfo
	for _, rev := range revList.Revisions {
		revInfo := &RevisionInfo{
			ID:          rev.Id,
			Size:        rev.Size,
			MimeType:    rev.MimeType,
			KeepForever: rev.KeepForever,
			Published:   rev.Published,
		}

		if rev.ModifiedTime != "" {
			if t, err := time.Parse(time.RFC3339, rev.ModifiedTime); err == nil {
				revInfo.ModifiedTime = t
			}
		}

		if rev.LastModifyingUser != nil {
			revInfo.ModifiedBy = rev.LastModifyingUser.DisplayName
			if revInfo.ModifiedBy == "" {
				revInfo.ModifiedBy = rev.LastModifyingUser.EmailAddress
			}
		}

		revisions = append(revisions, revInfo)
	}

	return revisions, nil
}

// GetRevision gets a specific revision of a file
func (ds *DriveService) GetRevision(fileID, revisionID string) (*RevisionInfo, error) {
	rev, err := ds.Service.Revisions.Get(fileID, revisionID).
		Fields("id, modifiedTime, size, mimeType, lastModifyingUser, keepForever, published").
		Do()
	if err != nil {
		return nil, fmt.Errorf("unable to get revision: %v", err)
	}

	revInfo := &RevisionInfo{
		ID:          rev.Id,
		Size:        rev.Size,
		MimeType:    rev.MimeType,
		KeepForever: rev.KeepForever,
		Published:   rev.Published,
	}

	if rev.ModifiedTime != "" {
		if t, err := time.Parse(time.RFC3339, rev.ModifiedTime); err == nil {
			revInfo.ModifiedTime = t
		}
	}

	if rev.LastModifyingUser != nil {
		revInfo.ModifiedBy = rev.LastModifyingUser.DisplayName
		if revInfo.ModifiedBy == "" {
			revInfo.ModifiedBy = rev.LastModifyingUser.EmailAddress
		}
	}

	return revInfo, nil
}

// DriveActivityInfo represents a drive activity event
type DriveActivityInfo struct {
	Timestamp    time.Time
	ActionType   string
	ActionDetail string
	Actors       []string
	Targets      []string
	TargetTitles []string
}

// QueryDriveActivity queries the Drive Activity API for recent activities
func QueryDriveActivity(activityService *driveactivity.Service, daysBack int, maxResults int64) ([]*DriveActivityInfo, error) {
	// Build the request
	pageSize := int64(100) // API max per page
	if maxResults > 0 && maxResults < pageSize {
		pageSize = maxResults
	}

	req := &driveactivity.QueryDriveActivityRequest{
		PageSize: pageSize,
	}

	// Add time filter if specified
	if daysBack > 0 {
		cutoffTime := time.Now().AddDate(0, 0, -daysBack)
		req.Filter = fmt.Sprintf("time >= \"%s\"", cutoffTime.Format(time.RFC3339))
	}

	var activities []*DriveActivityInfo
	pageToken := ""
	pageCount := 0
	maxPagesPerBatch := 90 // Stay under 100 queries/minute limit

	// Fetch all pages until we reach maxResults or no more pages
	for {
		if pageToken != "" {
			req.PageToken = pageToken
		}

		// Query activities with retry logic for rate limiting
		var resp *driveactivity.QueryDriveActivityResponse
		var err error
		maxRetries := 3
		baseDelay := 2 * time.Second

		for retry := 0; retry <= maxRetries; retry++ {
			resp, err = activityService.Activity.Query(req).Do()
			if err == nil {
				break
			}

			// Check if it's a rate limit error
			if strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "rateLimitExceeded") {
				if retry < maxRetries {
					// Exponential backoff: 2s, 4s, 8s
					delay := baseDelay * time.Duration(1<<uint(retry))
					fmt.Fprintf(os.Stderr, "⚠ Rate limit hit. Waiting %v before retry %d/%d...\n", delay, retry+1, maxRetries)
					time.Sleep(delay)
					continue
				}
			}
			return nil, fmt.Errorf("unable to query drive activity: %v", err)
		}

		pageCount++

		// Rate limiting: if we've made many requests, pause
		if pageCount%maxPagesPerBatch == 0 {
			fmt.Fprintf(os.Stderr, "⏸  Fetched %d pages (%d activities). Pausing 60s to respect rate limits...\n",
				pageCount, len(activities))
			time.Sleep(60 * time.Second)
		}

		// Process activities from this page
		for _, activity := range resp.Activities {
		activityInfo := &DriveActivityInfo{
			Actors:       []string{},
			Targets:      []string{},
			TargetTitles: []string{},
		}

		// Parse timestamp
		if activity.Timestamp != "" {
			if t, err := time.Parse(time.RFC3339Nano, activity.Timestamp); err == nil {
				activityInfo.Timestamp = t
			}
		}

		// Parse actors
		if activity.Actors != nil {
			for _, actor := range activity.Actors {
				if actor.User != nil && actor.User.KnownUser != nil {
					if actor.User.KnownUser.PersonName != "" {
						activityInfo.Actors = append(activityInfo.Actors, actor.User.KnownUser.PersonName)
					}
				} else if actor.Administrator != nil {
					activityInfo.Actors = append(activityInfo.Actors, "Administrator")
				} else if actor.Anonymous != nil {
					activityInfo.Actors = append(activityInfo.Actors, "Anonymous")
				}
			}
		}

		// Parse targets
		if activity.Targets != nil {
			for _, target := range activity.Targets {
				if target.DriveItem != nil {
					// Try Title first (this is the actual file name)
					if target.DriveItem.Title != "" {
						activityInfo.TargetTitles = append(activityInfo.TargetTitles, target.DriveItem.Title)
					} else if target.DriveItem.Name != "" {
						// Name is the resource ID, use only if Title is not available
						// Extract just the ID part for display
						name := target.DriveItem.Name
						if len(name) > 6 && name[:6] == "items/" {
							name = name[6:] // Remove "items/" prefix
						}
						activityInfo.TargetTitles = append(activityInfo.TargetTitles, fmt.Sprintf("<ID: %s>", name))
					}
					// Store the resource name/ID for reference
					if target.DriveItem.Name != "" {
						activityInfo.Targets = append(activityInfo.Targets, target.DriveItem.Name)
					}
				}
				if target.FileComment != nil && target.FileComment.Parent != nil {
					activityInfo.Targets = append(activityInfo.Targets, "Comment")
				}
			}
		}

		// Parse primary action
		if activity.PrimaryActionDetail != nil {
			activityInfo.ActionType, activityInfo.ActionDetail = parsePrimaryAction(activity.PrimaryActionDetail)
		}

		activities = append(activities, activityInfo)

			// Check if we've reached maxResults
			if maxResults > 0 && int64(len(activities)) >= maxResults {
				return activities, nil
			}
		}

		// Check if there are more pages
		pageToken = resp.NextPageToken
		if pageToken == "" {
			// No more pages
			break
		}

		// Check if we've reached maxResults
		if maxResults > 0 && int64(len(activities)) >= maxResults {
			break
		}
	}

	return activities, nil
}

// parsePrimaryAction extracts action type and details from PrimaryActionDetail
func parsePrimaryAction(action *driveactivity.ActionDetail) (string, string) {
	if action.Create != nil {
		if action.Create.New != nil {
			return "Create", "Created new item"
		}
		if action.Create.Upload != nil {
			return "Upload", "Uploaded file"
		}
		if action.Create.Copy != nil {
			return "Copy", "Copied file"
		}
	}

	if action.Edit != nil {
		return "Edit", "Edited file"
	}

	if action.Move != nil {
		detail := "Moved"
		if action.Move.AddedParents != nil && len(action.Move.AddedParents) > 0 {
			detail += " to new location"
		}
		if action.Move.RemovedParents != nil && len(action.Move.RemovedParents) > 0 {
			detail += " from old location"
		}
		return "Move", detail
	}

	if action.Rename != nil {
		detail := "Renamed"
		if action.Rename.OldTitle != "" && action.Rename.NewTitle != "" {
			detail = fmt.Sprintf("Renamed from '%s' to '%s'", action.Rename.OldTitle, action.Rename.NewTitle)
		}
		return "Rename", detail
	}

	if action.Delete != nil {
		deleteType := "Deleted"
		if action.Delete.Type == "TRASH" {
			deleteType = "Moved to trash"
		} else if action.Delete.Type == "PERMANENT_DELETE" {
			deleteType = "Permanently deleted"
		}
		return "Delete", deleteType
	}

	if action.Restore != nil {
		return "Restore", "Restored from trash"
	}

	if action.PermissionChange != nil {
		detail := "Changed permissions"
		if action.PermissionChange.AddedPermissions != nil && len(action.PermissionChange.AddedPermissions) > 0 {
			detail = "Added permissions"
		}
		if action.PermissionChange.RemovedPermissions != nil && len(action.PermissionChange.RemovedPermissions) > 0 {
			detail = "Removed permissions"
		}
		return "Permission", detail
	}

	if action.Comment != nil {
		if action.Comment.Post != nil {
			return "Comment", "Posted comment"
		}
		if action.Comment.Assignment != nil {
			return "Comment", "Assigned task"
		}
	}

	if action.DlpChange != nil {
		return "DLP", "Data loss prevention change"
	}

	if action.Reference != nil {
		return "Reference", "Referenced in another document"
	}

	if action.SettingsChange != nil {
		return "Settings", "Changed settings"
	}

	return "Unknown", "Unknown action"
}
