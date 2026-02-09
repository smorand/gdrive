// Package cli provides the command-line interface implementation.
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	driveapi "google.golang.org/api/drive/v3"

	"gdrive/internal/auth"
	"gdrive/internal/drive"
)

// Command flags
var (
	overwriteFlag bool
	useIDFlag     bool
	maxResults    int64
	fileTypeFlag  string
	parallelFlag  int
	newOnlyFlag   bool
	parentFlag    string
	roleFlag      string
	notifyFlag    bool
	messageFlag   string
	daysBackFlag  int
)

// Global config and flags
var (
	configDirFlag       string
	credentialsPathFlag string
	globalConfig        *auth.Config
)

// SetupRootCommand configures the root command with global flags.
func SetupRootCommand(rootCmd *cobra.Command) {
	rootCmd.PersistentFlags().StringVar(&configDirFlag, "config-dir", "",
		"Config directory (default: $HOME/.gdrive, env: GDRIVE_CONFIG_DIR)")
	rootCmd.PersistentFlags().StringVar(&credentialsPathFlag, "credentials", "",
		"Path to credentials.json file (env: GDRIVE_CREDENTIALS_PATH)")

	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		// Initialize global config with priority: CLI flags > env vars > defaults
		globalConfig = auth.NewConfig(configDirFlag, credentialsPathFlag)
	}
}

// FileCmd returns the file command.
func FileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "file",
		Short: "File operations",
		Long:  "Commands for uploading and downloading files",
	}

	cmd.AddCommand(fileDownloadCmd())
	cmd.AddCommand(fileUploadCmd())
	cmd.AddCommand(fileDeleteCmd())
	cmd.AddCommand(fileRenameCmd())
	cmd.AddCommand(fileMoveCmd())
	cmd.AddCommand(fileCopyCmd())
	cmd.AddCommand(fileInfoCmd())
	cmd.AddCommand(fileShareCmd())
	cmd.AddCommand(fileSharePublicCmd())
	cmd.AddCommand(filePermissionsCmd())
	cmd.AddCommand(fileRemovePermissionCmd())
	cmd.AddCommand(fileRemovePublicCmd())

	return cmd
}

// FolderCmd returns the folder command.
func FolderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "folder",
		Short: "Folder operations",
		Long:  "Commands for creating, uploading, downloading, and listing folders",
	}

	cmd.AddCommand(folderCreateCmd())
	cmd.AddCommand(folderUploadCmd())
	cmd.AddCommand(folderDownloadCmd())
	cmd.AddCommand(folderListCmd())

	return cmd
}

// SearchCmd returns the search command.
func SearchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search QUERY",
		Short: "Search for files and folders on Google Drive",
		Long: `Search for files and folders on Google Drive. Displays file name, ID, and last modification date.

File types can be shortcuts (image, audio, video, prez, doc, spreadsheet, txt, pdf, folder)
or explicit MIME types (e.g., image/jpeg, application/pdf).

Examples:
  gdrive search report
  gdrive search "budget 2024" --max 20
  gdrive search Passeport --type image,pdf
  gdrive search Passeport --type pdf,image/jpeg
  gdrive search "My Project" --type folder
  gdrive search contract --type doc -m 10`,
		Args: cobra.ExactArgs(1),
		RunE: runSearch,
	}

	cmd.Flags().Int64VarP(&maxResults, "max", "m", 50, "Maximum number of results")
	cmd.Flags().StringVarP(&fileTypeFlag, "type", "t", "", "Filter by file types (comma-separated)")

	return cmd
}

// ActivityCmd returns the activity command.
func ActivityCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "activity",
		Short: "View activity and revision history",
		Long:  "Commands for viewing recent changes and file revision history",
	}

	cmd.AddCommand(activityChangesCmd())
	cmd.AddCommand(activityRevisionsCmd())
	cmd.AddCommand(activityDeletedCmd())
	cmd.AddCommand(activityHistoryCmd())

	return cmd
}

// getDriveService returns an authenticated drive service.
func getDriveService() (*drive.Service, error) {
	srv, err := auth.GetAuthenticatedService(globalConfig)
	if err != nil {
		return nil, fmt.Errorf("authentication error: %v", err)
	}
	return drive.NewService(srv), nil
}

func confirmOverwrite(localPath string, remoteSize int64) bool {
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		return true
	}

	stat, err := os.Stat(localPath)
	if err != nil {
		return true
	}

	fmt.Println("\nFile Comparison:")
	fmt.Printf("%-10s %-20s %-30s\n", "Location", "Size (bytes)", "Modified")
	fmt.Printf("%-10s %-20d %-30s\n", "Local", stat.Size(), stat.ModTime().Format(time.RFC3339))
	if remoteSize > 0 {
		fmt.Printf("%-10s %-20d %-30s\n", "Remote", remoteSize, "")
	}

	fmt.Printf("\nOverwrite %s? (y/N): ", localPath)
	var response string
	fmt.Scanln(&response)
	return strings.ToLower(response) == "y" || strings.ToLower(response) == "yes"
}

func printItemRow(ds *drive.Service, item *driveapi.File) {
	itemType := "📄 File"
	if ds.IsFolder(item) {
		itemType = "📁 Folder"
	}

	modifiedStr := "N/A"
	if item.ModifiedTime != "" {
		modTime, err := time.Parse(time.RFC3339, item.ModifiedTime)
		if err == nil {
			modifiedStr = modTime.Format("2006-01-02 15:04")
		}
	}

	sizeStr := "-"
	if !ds.IsFolder(item) && item.Size > 0 {
		sizeStr = formatSize(item.Size)
	}

	// Truncate name if too long
	name := item.Name
	if len(name) > 38 {
		name = name[:35] + "..."
	}

	id := item.Id

	fmt.Printf("%-10s %-40s %-44s %-20s %12s\n", itemType, name, id, modifiedStr, sizeStr)
}

func formatSize(sizeBytes int64) string {
	size := float64(sizeBytes)
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}

	for _, unit := range units {
		if size < 1024.0 {
			return fmt.Sprintf("%.1f %s", size, unit)
		}
		size /= 1024.0
	}

	return fmt.Sprintf("%.1f PB", size)
}

// File command implementations

func fileDownloadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "download REMOTE_FILE [LOCAL_FOLDER]",
		Short: "Download a file from Google Drive",
		Long: `Download a file from Google Drive.

Examples:
  gdrive file download Parameters/file.txt
  gdrive file download Parameters/file.txt ./downloads
  gdrive file download Parameters/file.txt ./downloads --overwrite
  gdrive file download 1a2b3c4d5e --id`,
		Args: cobra.RangeArgs(1, 2),
		RunE: runFileDownload,
	}

	cmd.Flags().BoolVar(&overwriteFlag, "overwrite", false, "Overwrite without asking")
	cmd.Flags().BoolVar(&useIDFlag, "id", false, "Treat remote_file as a Drive file ID")

	return cmd
}

func fileUploadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upload LOCAL_FILE REMOTE_FOLDER",
		Short: "Upload a file to Google Drive",
		Long: `Upload a file to Google Drive. If file exists, creates a new version.

Examples:
  gdrive file upload ./myfile.txt Parameters/bin
  gdrive file upload /path/to/file.pdf Documents
  gdrive file upload ./myfile.txt 1a2b3c4d5e --id`,
		Args: cobra.ExactArgs(2),
		RunE: runFileUpload,
	}

	cmd.Flags().BoolVar(&useIDFlag, "id", false, "Treat remote_folder as a Drive folder ID")

	return cmd
}

func fileDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete FILE",
		Short: "Delete a file from Google Drive",
		Long: `Delete a file from Google Drive.

Examples:
  gdrive file delete Parameters/file.txt
  gdrive file delete 1a2b3c4d5e --id`,
		Args: cobra.ExactArgs(1),
		RunE: runFileDelete,
	}

	cmd.Flags().BoolVar(&useIDFlag, "id", false, "Treat FILE as a Drive file ID")

	return cmd
}

func fileRenameCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rename FILE NEW_NAME",
		Short: "Rename a file on Google Drive",
		Long: `Rename a file on Google Drive.

Examples:
  gdrive file rename Parameters/old.txt new.txt
  gdrive file rename 1a2b3c4d5e new_name.txt --id`,
		Args: cobra.ExactArgs(2),
		RunE: runFileRename,
	}

	cmd.Flags().BoolVar(&useIDFlag, "id", false, "Treat FILE as a Drive file ID")

	return cmd
}

func fileMoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "move FILE TARGET_FOLDER",
		Short: "Move a file to a different folder",
		Long: `Move a file to a different folder on Google Drive.

Examples:
  gdrive file move Parameters/file.txt Documents
  gdrive file move 1a2b3c4d5e 1xyz789 --id`,
		Args: cobra.ExactArgs(2),
		RunE: runFileMove,
	}

	cmd.Flags().BoolVar(&useIDFlag, "id", false, "Treat FILE and TARGET_FOLDER as Drive IDs")

	return cmd
}

func fileCopyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "copy FILE [NEW_NAME]",
		Short: "Copy a file on Google Drive",
		Long: `Copy a file on Google Drive. Optionally provide a new name and/or parent folder.

Examples:
  gdrive file copy Parameters/file.txt
  gdrive file copy Parameters/file.txt "Copy of file.txt"
  gdrive file copy 1a2b3c4d5e --id
  gdrive file copy Parameters/file.txt --parent Documents`,
		Args: cobra.RangeArgs(1, 2),
		RunE: runFileCopy,
	}

	cmd.Flags().BoolVar(&useIDFlag, "id", false, "Treat FILE as a Drive file ID")
	cmd.Flags().StringVar(&parentFlag, "parent", "", "Parent folder path or ID for the copy")

	return cmd
}

func fileInfoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info FILE",
		Short: "Display detailed information about a file",
		Long: `Display detailed information about a file including path, size, dates, and owners.

Examples:
  gdrive file info Parameters/file.txt
  gdrive file info 1a2b3c4d5e --id`,
		Args: cobra.ExactArgs(1),
		RunE: runFileInfo,
	}

	cmd.Flags().BoolVar(&useIDFlag, "id", false, "Treat FILE as a Drive file ID")

	return cmd
}

func fileShareCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "share FILE EMAIL",
		Short: "Share a file with a user",
		Long: `Share a file with a user via email.

Examples:
  gdrive file share Parameters/file.txt user@example.com
  gdrive file share Parameters/file.txt user@example.com --role writer
  gdrive file share 1a2b3c4d5e user@example.com --id --no-notify`,
		Args: cobra.ExactArgs(2),
		RunE: runFileShare,
	}

	cmd.Flags().BoolVar(&useIDFlag, "id", false, "Treat FILE as a Drive file ID")
	cmd.Flags().StringVar(&roleFlag, "role", "reader", "Permission role (reader, writer, commenter)")
	cmd.Flags().BoolVar(&notifyFlag, "no-notify", false, "Do not send notification email")
	cmd.Flags().StringVar(&messageFlag, "message", "", "Custom message for the notification email")

	return cmd
}

func fileSharePublicCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "share-public FILE",
		Short: "Share a file with anyone who has the link",
		Long: `Share a file with anyone who has the link.

Examples:
  gdrive file share-public Parameters/file.txt
  gdrive file share-public Parameters/file.txt --role writer
  gdrive file share-public 1a2b3c4d5e --id`,
		Args: cobra.ExactArgs(1),
		RunE: runFileSharePublic,
	}

	cmd.Flags().BoolVar(&useIDFlag, "id", false, "Treat FILE as a Drive file ID")
	cmd.Flags().StringVar(&roleFlag, "role", "reader", "Permission role (reader, writer, commenter)")

	return cmd
}

func filePermissionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "permissions FILE",
		Short: "List all permissions for a file",
		Long: `List all permissions for a file.

Examples:
  gdrive file permissions Parameters/file.txt
  gdrive file permissions 1a2b3c4d5e --id`,
		Args: cobra.ExactArgs(1),
		RunE: runFilePermissions,
	}

	cmd.Flags().BoolVar(&useIDFlag, "id", false, "Treat FILE as a Drive file ID")

	return cmd
}

func fileRemovePermissionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove-permission FILE PERMISSION_ID",
		Short: "Remove a specific permission from a file",
		Long: `Remove a specific permission from a file. Use 'permissions' command to get permission IDs.

Examples:
  gdrive file remove-permission Parameters/file.txt 12345678
  gdrive file remove-permission 1a2b3c4d5e 12345678 --id`,
		Args: cobra.ExactArgs(2),
		RunE: runFileRemovePermission,
	}

	cmd.Flags().BoolVar(&useIDFlag, "id", false, "Treat FILE as a Drive file ID")

	return cmd
}

func fileRemovePublicCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove-public FILE",
		Short: "Remove public access from a file",
		Long: `Remove public access (anyone with the link) from a file.

Examples:
  gdrive file remove-public Parameters/file.txt
  gdrive file remove-public 1a2b3c4d5e --id`,
		Args: cobra.ExactArgs(1),
		RunE: runFileRemovePublic,
	}

	cmd.Flags().BoolVar(&useIDFlag, "id", false, "Treat FILE as a Drive file ID")

	return cmd
}

// Folder command implementations

func folderCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create REMOTE_FOLDER",
		Short: "Create a folder path on Google Drive (like mkdir -p)",
		Long: `Create a folder path on Google Drive (like mkdir -p).

Examples:
  gdrive folder create Parameters/bin
  gdrive folder create Documents/Projects/2024`,
		Args: cobra.ExactArgs(1),
		RunE: runFolderCreate,
	}
}

func folderUploadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upload LOCAL_SRC REMOTE_FOLDER",
		Short: "Upload a folder recursively to Google Drive",
		Long: `Upload a folder recursively to Google Drive. Creates new versions for existing files.

Examples:
  gdrive folder upload ./my_project Parameters/Projects
  gdrive folder upload /path/to/folder Documents/Backup
  gdrive folder upload ./my_project 1a2b3c4d5e --id`,
		Args: cobra.ExactArgs(2),
		RunE: runFolderUpload,
	}

	cmd.Flags().BoolVar(&useIDFlag, "id", false, "Treat remote_folder as a Drive folder ID")

	return cmd
}

func folderDownloadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "download REMOTE_FOLDER LOCAL_FOLDER",
		Short: "Download a folder recursively from Google Drive",
		Long: `Download a folder recursively from Google Drive.

Examples:
  gdrive folder download Parameters/bin ./downloads
  gdrive folder download Documents/Projects ./backup --overwrite
  gdrive folder download 1a2b3c4d5e ./downloads --id
  gdrive folder download Documents ./backup --parallel 10
  gdrive folder download Documents ./backup --new-only
  gdrive folder download Documents ./backup --new-only --overwrite`,
		Args: cobra.ExactArgs(2),
		RunE: runFolderDownload,
	}

	cmd.Flags().BoolVar(&overwriteFlag, "overwrite", false, "Overwrite without asking")
	cmd.Flags().BoolVar(&useIDFlag, "id", false, "Treat remote_folder as a Drive folder ID")
	cmd.Flags().IntVarP(&parallelFlag, "parallel", "p", 5, "Number of parallel downloads (1-20)")
	cmd.Flags().BoolVar(&newOnlyFlag, "new-only", false, "Only download new or newer files from Drive")

	return cmd
}

func folderListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list REMOTE_FOLDER",
		Short: "List contents of a folder on Google Drive",
		Long: `List contents of a folder on Google Drive. Displays file name, ID, and last modification date.

Examples:
  gdrive folder list Parameters/bin
  gdrive folder list Documents
  gdrive folder list 1a2b3c4d5e --id`,
		Args: cobra.ExactArgs(1),
		RunE: runFolderList,
	}

	cmd.Flags().BoolVar(&useIDFlag, "id", false, "Treat remote_folder as a Drive folder ID")

	return cmd
}

// Activity command implementations

func activityChangesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "changes",
		Short: "List recent changes to files",
		Long: `List recent changes to files in Google Drive.
Shows additions, modifications, and removals.

Examples:
  gdrive activity changes
  gdrive activity changes --max 20`,
		Args: cobra.NoArgs,
		RunE: runActivityChanges,
	}

	cmd.Flags().Int64VarP(&maxResults, "max", "m", 50, "Maximum number of changes to show")

	return cmd
}

func activityRevisionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revisions FILE",
		Short: "List revision history for a file",
		Long: `List all revisions for a specific file.
Shows modification time, size, and who made the change.

Examples:
  gdrive activity revisions Parameters/file.txt
  gdrive activity revisions 1a2b3c4d5e --id`,
		Args: cobra.ExactArgs(1),
		RunE: runActivityRevisions,
	}

	cmd.Flags().BoolVar(&useIDFlag, "id", false, "Treat FILE as a Drive file ID")

	return cmd
}

func activityDeletedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deleted",
		Short: "List recently deleted files",
		Long: `List files that have been deleted (moved to trash).
Shows file name, deletion time, size, and who deleted it.

Examples:
  gdrive activity deleted
  gdrive activity deleted --days 7
  gdrive activity deleted --days 30 --max 100`,
		Args: cobra.NoArgs,
		RunE: runActivityDeleted,
	}

	cmd.Flags().IntVar(&daysBackFlag, "days", 7, "Number of days back to search for deleted files")
	cmd.Flags().Int64VarP(&maxResults, "max", "m", 100, "Maximum number of deleted files to show")

	return cmd
}

func activityHistoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "List detailed activity history",
		Long: `List detailed activity history using Drive Activity API.
Shows all activities including permanent deletions, edits, moves, and permission changes.

Note: The API may have limits on historical data retention.
For very large result sets, increase --max (e.g., --max 1000).

Examples:
  gdrive activity history
  gdrive activity history --days 14
  gdrive activity history --days 30 --max 500
  gdrive activity history --days 7 --max 1000`,
		Args: cobra.NoArgs,
		RunE: runActivityHistory,
	}

	cmd.Flags().IntVar(&daysBackFlag, "days", 7, "Number of days back to show activity history")
	cmd.Flags().Int64VarP(&maxResults, "max", "m", 100, "Maximum number of activities to show")

	return cmd
}

// Run functions for file commands

func runFileDownload(cmd *cobra.Command, args []string) error {
	ds, err := getDriveService()
	if err != nil {
		return err
	}

	remoteFile := args[0]
	localFolder := "."
	if len(args) > 1 {
		localFolder = args[1]
	}

	var fileID string
	var filename string

	if useIDFlag {
		// Use remote_file as file ID directly
		fileID = remoteFile
		fileItem, err := ds.API.Files.Get(fileID).Fields("id, name, size, modifiedTime").Do()
		if err != nil {
			return fmt.Errorf("file not found: %v", err)
		}
		filename = fileItem.Name
	} else {
		// Parse remote path
		parts := strings.Split(strings.Trim(remoteFile, "/"), "/")
		if len(parts) < 1 {
			return fmt.Errorf("invalid remote file path")
		}

		filename = parts[len(parts)-1]
		var folderPath string
		if len(parts) > 1 {
			folderPath = strings.Join(parts[:len(parts)-1], "/")
		}

		// Resolve parent folder
		parentID := "root"
		if folderPath != "" {
			parentID, err = ds.ResolvePath(folderPath, true)
			if err != nil {
				return err
			}
		}

		// Find file
		fileItem, err := ds.FindFile(filename, parentID)
		if err != nil {
			return err
		}
		if fileItem == nil {
			return fmt.Errorf("file not found: %s", remoteFile)
		}
		fileID = fileItem.Id
	}

	// Determine local path
	localPath := filepath.Join(localFolder, filename)

	// Check overwrite
	if !overwriteFlag {
		if _, err := os.Stat(localPath); err == nil {
			if !confirmOverwrite(localPath, 0) {
				color.Yellow("Download cancelled")
				return nil
			}
		}
	}

	// Download file
	if err := ds.DownloadFile(fileID, localPath, true, true); err != nil {
		return err
	}

	color.Green("Downloaded: %s", localPath)
	return nil
}

func runFileUpload(cmd *cobra.Command, args []string) error {
	ds, err := getDriveService()
	if err != nil {
		return err
	}

	localFile := args[0]
	remoteFolder := args[1]

	// Check local file exists
	if _, err := os.Stat(localFile); os.IsNotExist(err) {
		return fmt.Errorf("local file not found: %s", localFile)
	}

	stat, err := os.Stat(localFile)
	if err != nil {
		return err
	}
	if stat.IsDir() {
		return fmt.Errorf("not a file: %s", localFile)
	}

	// Get folder ID
	var folderID string
	if useIDFlag {
		folderID = remoteFolder
	} else {
		folderID, err = ds.ResolvePath(remoteFolder, true)
		if err != nil {
			color.Red("Remote folder does not exist: %s", remoteFolder)
			fmt.Println("Use 'gdrive folder create' to create it first")
			return err
		}
	}

	// Upload file
	if _, err := ds.UploadFile(localFile, folderID, true); err != nil {
		return err
	}

	color.Green("Uploaded: %s -> %s/%s", localFile, remoteFolder, filepath.Base(localFile))
	return nil
}

func runFileDelete(cmd *cobra.Command, args []string) error {
	ds, err := getDriveService()
	if err != nil {
		return err
	}

	filePath := args[0]

	// Get file ID
	var fileID string
	if useIDFlag {
		fileID = filePath
	} else {
		// Parse path to get folder and filename
		dir := filepath.Dir(filePath)
		filename := filepath.Base(filePath)

		parentID, err := ds.ResolvePath(dir, true)
		if err != nil {
			return fmt.Errorf("parent folder not found: %v", err)
		}

		file, err := ds.FindFile(filename, parentID)
		if err != nil {
			return err
		}
		if file == nil {
			return fmt.Errorf("file not found: %s", filePath)
		}
		fileID = file.Id
	}

	// Confirm deletion
	fmt.Printf("Are you sure you want to delete this file? (y/N): ")
	var response string
	fmt.Scanln(&response)
	if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
		color.Yellow("Deletion cancelled")
		return nil
	}

	// Delete file
	if err := ds.DeleteFile(fileID); err != nil {
		return err
	}

	color.Green("✓ File deleted successfully")
	return nil
}

func runFileRename(cmd *cobra.Command, args []string) error {
	ds, err := getDriveService()
	if err != nil {
		return err
	}

	filePath := args[0]
	newName := args[1]

	// Get file ID
	var fileID string
	if useIDFlag {
		fileID = filePath
	} else {
		// Parse path to get folder and filename
		dir := filepath.Dir(filePath)
		filename := filepath.Base(filePath)

		parentID, err := ds.ResolvePath(dir, true)
		if err != nil {
			return fmt.Errorf("parent folder not found: %v", err)
		}

		file, err := ds.FindFile(filename, parentID)
		if err != nil {
			return err
		}
		if file == nil {
			return fmt.Errorf("file not found: %s", filePath)
		}
		fileID = file.Id
	}

	// Rename file
	renamedFile, err := ds.RenameFile(fileID, newName)
	if err != nil {
		return err
	}

	color.Green("✓ File renamed successfully")
	fmt.Printf("  Name: %s\n", renamedFile.Name)
	fmt.Printf("  ID:   %s\n", renamedFile.Id)
	if renamedFile.WebViewLink != "" {
		fmt.Printf("  Link: %s\n", renamedFile.WebViewLink)
	}
	return nil
}

func runFileMove(cmd *cobra.Command, args []string) error {
	ds, err := getDriveService()
	if err != nil {
		return err
	}

	filePath := args[0]
	targetFolder := args[1]

	// Get file ID
	var fileID string
	if useIDFlag {
		fileID = filePath
	} else {
		// Parse path to get folder and filename
		dir := filepath.Dir(filePath)
		filename := filepath.Base(filePath)

		parentID, err := ds.ResolvePath(dir, true)
		if err != nil {
			return fmt.Errorf("parent folder not found: %v", err)
		}

		file, err := ds.FindFile(filename, parentID)
		if err != nil {
			return err
		}
		if file == nil {
			return fmt.Errorf("file not found: %s", filePath)
		}
		fileID = file.Id
	}

	// Get target folder ID
	var targetFolderID string
	if useIDFlag {
		targetFolderID = targetFolder
	} else {
		targetFolderID, err = ds.ResolvePath(targetFolder, true)
		if err != nil {
			return fmt.Errorf("target folder not found: %v", err)
		}
	}

	// Move file
	movedFile, err := ds.MoveFile(fileID, targetFolderID)
	if err != nil {
		return err
	}

	color.Green("✓ File moved successfully")
	fmt.Printf("  Name: %s\n", movedFile.Name)
	fmt.Printf("  ID:   %s\n", movedFile.Id)
	return nil
}

func runFileCopy(cmd *cobra.Command, args []string) error {
	ds, err := getDriveService()
	if err != nil {
		return err
	}

	filePath := args[0]
	var newName string
	if len(args) > 1 {
		newName = args[1]
	}

	// Get file ID
	var fileID string
	if useIDFlag {
		fileID = filePath
	} else {
		// Parse path to get folder and filename
		dir := filepath.Dir(filePath)
		filename := filepath.Base(filePath)

		parentID, err := ds.ResolvePath(dir, true)
		if err != nil {
			return fmt.Errorf("parent folder not found: %v", err)
		}

		file, err := ds.FindFile(filename, parentID)
		if err != nil {
			return err
		}
		if file == nil {
			return fmt.Errorf("file not found: %s", filePath)
		}
		fileID = file.Id
	}

	// Get parent folder ID if specified
	var parentFolderID string
	if parentFlag != "" {
		if useIDFlag {
			parentFolderID = parentFlag
		} else {
			parentFolderID, err = ds.ResolvePath(parentFlag, true)
			if err != nil {
				return fmt.Errorf("parent folder not found: %v", err)
			}
		}
	}

	// Copy file
	copiedFile, err := ds.CopyFile(fileID, drive.CopyOptions{
		NewName:        newName,
		ParentFolderID: parentFolderID,
	})
	if err != nil {
		return err
	}

	color.Green("✓ File copied successfully")
	fmt.Printf("  Name: %s\n", copiedFile.Name)
	fmt.Printf("  ID:   %s\n", copiedFile.Id)
	if copiedFile.WebViewLink != "" {
		fmt.Printf("  Link: %s\n", copiedFile.WebViewLink)
	}
	return nil
}

func runFileInfo(cmd *cobra.Command, args []string) error {
	ds, err := getDriveService()
	if err != nil {
		return err
	}

	filePath := args[0]

	// Get file ID
	var fileID string
	if useIDFlag {
		fileID = filePath
	} else {
		// Parse path to get folder and filename
		dir := filepath.Dir(filePath)
		filename := filepath.Base(filePath)

		parentID, err := ds.ResolvePath(dir, true)
		if err != nil {
			return fmt.Errorf("parent folder not found: %v", err)
		}

		file, err := ds.FindFile(filename, parentID)
		if err != nil {
			return err
		}
		if file == nil {
			return fmt.Errorf("file not found: %s", filePath)
		}
		fileID = file.Id
	}

	// Get file info
	fileInfo, err := ds.GetFileInfo(fileID)
	if err != nil {
		return err
	}

	// Display file information
	color.Cyan("\n📄 File Information:")
	fmt.Printf("  Name:     %s\n", fileInfo.Name)
	fmt.Printf("  ID:       %s\n", fileInfo.ID)
	fmt.Printf("  Type:     %s\n", fileInfo.MimeType)
	if fileInfo.Size > 0 {
		fmt.Printf("  Size:     %s\n", formatSize(fileInfo.Size))
	}
	fmt.Printf("  Created:  %s\n", fileInfo.CreatedTime)
	fmt.Printf("  Modified: %s\n", fileInfo.ModifiedTime)
	if fileInfo.WebViewLink != "" {
		fmt.Printf("  Link:     %s\n", fileInfo.WebViewLink)
	}
	if len(fileInfo.Owners) > 0 {
		ownerNames := make([]string, len(fileInfo.Owners))
		for i, owner := range fileInfo.Owners {
			if owner.DisplayName != "" {
				ownerNames[i] = owner.DisplayName
			} else if owner.EmailAddress != "" {
				ownerNames[i] = owner.EmailAddress
			} else {
				ownerNames[i] = "Unknown"
			}
		}
		fmt.Printf("  Owners:   %s\n", strings.Join(ownerNames, ", "))
	}

	// Display path
	if len(fileInfo.Path) > 0 {
		pathNames := make([]string, len(fileInfo.Path))
		for i, component := range fileInfo.Path {
			pathNames[i] = component.Name
		}
		fmt.Printf("  Path:     %s\n", strings.Join(pathNames, " / "))
	}

	return nil
}

func runFileShare(cmd *cobra.Command, args []string) error {
	ds, err := getDriveService()
	if err != nil {
		return err
	}

	filePath := args[0]
	email := args[1]

	// Get file ID
	var fileID string
	if useIDFlag {
		fileID = filePath
	} else {
		// Parse path to get folder and filename
		dir := filepath.Dir(filePath)
		filename := filepath.Base(filePath)

		parentID, err := ds.ResolvePath(dir, true)
		if err != nil {
			return fmt.Errorf("parent folder not found: %v", err)
		}

		file, err := ds.FindFile(filename, parentID)
		if err != nil {
			return err
		}
		if file == nil {
			return fmt.Errorf("file not found: %s", filePath)
		}
		fileID = file.Id
	}

	// Share file
	if err := ds.ShareFile(fileID, drive.ShareOptions{
		Email:   email,
		Role:    roleFlag,
		Notify:  !notifyFlag,
		Message: messageFlag,
	}); err != nil {
		return err
	}

	color.Green("✓ File shared successfully with %s as %s", email, roleFlag)
	return nil
}

func runFileSharePublic(cmd *cobra.Command, args []string) error {
	ds, err := getDriveService()
	if err != nil {
		return err
	}

	filePath := args[0]

	// Get file ID
	var fileID string
	if useIDFlag {
		fileID = filePath
	} else {
		// Parse path to get folder and filename
		dir := filepath.Dir(filePath)
		filename := filepath.Base(filePath)

		parentID, err := ds.ResolvePath(dir, true)
		if err != nil {
			return fmt.Errorf("parent folder not found: %v", err)
		}

		file, err := ds.FindFile(filename, parentID)
		if err != nil {
			return err
		}
		if file == nil {
			return fmt.Errorf("file not found: %s", filePath)
		}
		fileID = file.Id
	}

	// Share with anyone
	if err := ds.ShareWithAnyone(fileID, roleFlag); err != nil {
		return err
	}

	color.Green("✓ File is now shared with anyone who has the link as %s", roleFlag)
	return nil
}

func runFilePermissions(cmd *cobra.Command, args []string) error {
	ds, err := getDriveService()
	if err != nil {
		return err
	}

	filePath := args[0]

	// Get file ID
	var fileID string
	if useIDFlag {
		fileID = filePath
	} else {
		// Parse path to get folder and filename
		dir := filepath.Dir(filePath)
		filename := filepath.Base(filePath)

		parentID, err := ds.ResolvePath(dir, true)
		if err != nil {
			return fmt.Errorf("parent folder not found: %v", err)
		}

		file, err := ds.FindFile(filename, parentID)
		if err != nil {
			return err
		}
		if file == nil {
			return fmt.Errorf("file not found: %s", filePath)
		}
		fileID = file.Id
	}

	// List permissions
	permissions, err := ds.ListPermissions(fileID)
	if err != nil {
		return err
	}

	if len(permissions) == 0 {
		color.Yellow("No permissions found")
		return nil
	}

	color.Cyan("\n🔐 Permissions:")
	for _, perm := range permissions {
		permType := perm.Type
		role := perm.Role
		permID := perm.Id

		var displayInfo string
		switch permType {
		case "user":
			displayName := perm.DisplayName
			if displayName == "" {
				displayName = perm.EmailAddress
			}
			if displayName == "" {
				displayName = "Unknown"
			}
			email := perm.EmailAddress
			if email == "" {
				email = "N/A"
			}
			displayInfo = fmt.Sprintf("👤 User: %s (%s)", displayName, email)
		case "group":
			displayName := perm.DisplayName
			if displayName == "" {
				displayName = perm.EmailAddress
			}
			if displayName == "" {
				displayName = "Unknown"
			}
			email := perm.EmailAddress
			if email == "" {
				email = "N/A"
			}
			displayInfo = fmt.Sprintf("👥 Group: %s (%s)", displayName, email)
		case "domain":
			domain := perm.Domain
			if domain == "" {
				domain = "N/A"
			}
			displayInfo = fmt.Sprintf("🏢 Domain: %s", domain)
		case "anyone":
			displayInfo = "🌐 Anyone with the link"
		default:
			displayInfo = fmt.Sprintf("❓ Unknown type: %s", permType)
		}

		fmt.Printf("\n%s\n", displayInfo)
		fmt.Printf("   Role: %s\n", role)
		fmt.Printf("   Permission ID: %s\n", permID)
	}

	return nil
}

func runFileRemovePermission(cmd *cobra.Command, args []string) error {
	ds, err := getDriveService()
	if err != nil {
		return err
	}

	filePath := args[0]
	permissionID := args[1]

	// Get file ID
	var fileID string
	if useIDFlag {
		fileID = filePath
	} else {
		// Parse path to get folder and filename
		dir := filepath.Dir(filePath)
		filename := filepath.Base(filePath)

		parentID, err := ds.ResolvePath(dir, true)
		if err != nil {
			return fmt.Errorf("parent folder not found: %v", err)
		}

		file, err := ds.FindFile(filename, parentID)
		if err != nil {
			return err
		}
		if file == nil {
			return fmt.Errorf("file not found: %s", filePath)
		}
		fileID = file.Id
	}

	// Remove permission
	if err := ds.RemovePermission(fileID, permissionID); err != nil {
		return err
	}

	color.Green("✓ Permission removed successfully")
	return nil
}

func runFileRemovePublic(cmd *cobra.Command, args []string) error {
	ds, err := getDriveService()
	if err != nil {
		return err
	}

	filePath := args[0]

	// Get file ID
	var fileID string
	if useIDFlag {
		fileID = filePath
	} else {
		// Parse path to get folder and filename
		dir := filepath.Dir(filePath)
		filename := filepath.Base(filePath)

		parentID, err := ds.ResolvePath(dir, true)
		if err != nil {
			return fmt.Errorf("parent folder not found: %v", err)
		}

		file, err := ds.FindFile(filename, parentID)
		if err != nil {
			return err
		}
		if file == nil {
			return fmt.Errorf("file not found: %s", filePath)
		}
		fileID = file.Id
	}

	// Remove public access
	if err := ds.RemovePublicAccess(fileID); err != nil {
		return err
	}

	color.Green("✓ Public access removed successfully")
	return nil
}

// Run functions for folder commands

func runFolderCreate(cmd *cobra.Command, args []string) error {
	ds, err := getDriveService()
	if err != nil {
		return err
	}

	remoteFolder := args[0]
	if _, err := ds.CreateFolderPath(remoteFolder); err != nil {
		return err
	}

	color.Green("Folder path created: %s", remoteFolder)
	return nil
}

func runFolderUpload(cmd *cobra.Command, args []string) error {
	ds, err := getDriveService()
	if err != nil {
		return err
	}

	localSrc := args[0]
	remoteFolder := args[1]

	// Check local folder exists
	stat, err := os.Stat(localSrc)
	if os.IsNotExist(err) {
		return fmt.Errorf("local folder not found: %s", localSrc)
	}
	if !stat.IsDir() {
		return fmt.Errorf("not a folder: %s", localSrc)
	}

	// Get folder ID
	var folderID string
	if useIDFlag {
		folderID = remoteFolder
	} else {
		folderID, err = ds.ResolvePath(remoteFolder, true)
		if err != nil {
			color.Red("Remote folder does not exist: %s", remoteFolder)
			fmt.Println("Use 'gdrive folder create' to create it first")
			return err
		}
	}

	// Upload recursively
	if err := uploadFolderRecursive(ds, localSrc, folderID, remoteFolder); err != nil {
		return err
	}

	color.Green("Uploaded folder: %s -> %s", localSrc, remoteFolder)
	return nil
}

func uploadFolderRecursive(ds *drive.Service, localPath, parentID, remotePath string) error {
	entries, err := os.ReadDir(localPath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		itemPath := filepath.Join(localPath, entry.Name())

		if entry.IsDir() {
			// Create subfolder if doesn't exist
			subfolderItem, err := ds.FindFile(entry.Name(), parentID)
			if err != nil {
				return err
			}

			var subfolderID string
			if subfolderItem != nil && ds.IsFolder(subfolderItem) {
				subfolderID = subfolderItem.Id
			} else {
				// Create folder
				fileMetadata := &driveapi.File{
					Name:     entry.Name(),
					MimeType: "application/vnd.google-apps.folder",
					Parents:  []string{parentID},
				}
				folder, err := ds.API.Files.Create(fileMetadata).Fields("id").Do()
				if err != nil {
					return err
				}
				subfolderID = folder.Id
				fmt.Printf("Created folder: %s/%s\n", remotePath, entry.Name())
			}

			// Recurse into subfolder
			if err := uploadFolderRecursive(ds, itemPath, subfolderID, remotePath+"/"+entry.Name()); err != nil {
				return err
			}
		} else {
			// Upload file
			if _, err := ds.UploadFile(itemPath, parentID, true); err != nil {
				return err
			}
		}
	}

	return nil
}

func runFolderDownload(cmd *cobra.Command, args []string) error {
	ds, err := getDriveService()
	if err != nil {
		return err
	}

	remoteFolder := args[0]
	localFolder := args[1]

	// Validate parallel flag
	if parallelFlag < 1 || parallelFlag > 20 {
		return fmt.Errorf("parallel downloads must be between 1 and 20")
	}

	// Get folder ID
	var folderID string
	if useIDFlag {
		folderID = remoteFolder
	} else {
		folderID, err = ds.ResolvePath(remoteFolder, true)
		if err != nil {
			return fmt.Errorf("remote folder not found: %v", err)
		}
	}

	// Create local folder
	if err := os.MkdirAll(localFolder, 0755); err != nil {
		return err
	}

	// Download recursively
	if err := downloadFolderRecursive(ds, folderID, localFolder, overwriteFlag, parallelFlag, newOnlyFlag); err != nil {
		return err
	}

	color.Green("Downloaded folder: %s -> %s", remoteFolder, localFolder)
	return nil
}

func downloadFolderRecursive(ds *drive.Service, folderID, localPath string, overwrite bool, parallel int, newOnly bool) error {
	items, err := ds.ListFolder(folderID)
	if err != nil {
		return err
	}

	// First, process all folders recursively (sequential)
	for _, item := range items {
		if ds.IsFolder(item) {
			// Create local subfolder and recurse
			subfolderPath := filepath.Join(localPath, item.Name)
			if err := os.MkdirAll(subfolderPath, 0755); err != nil {
				return err
			}
			if err := downloadFolderRecursive(ds, item.Id, subfolderPath, overwrite, parallel, newOnly); err != nil {
				return err
			}
		}
	}

	// Then, download all files in parallel
	var filesToDownload []*driveapi.File
	for _, item := range items {
		if !ds.IsFolder(item) && !ds.IsGoogleWorkspaceFile(item) {
			filesToDownload = append(filesToDownload, item)
		} else if ds.IsGoogleWorkspaceFile(item) {
			// Skip Google Workspace files
			color.Yellow("Skipped Google Workspace file: %s (use export instead)", item.Name)
		}
	}

	// Download files in parallel with limited concurrency
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var errors []error

	for _, item := range filesToDownload {
		filePath := filepath.Join(localPath, item.Name)

		// Check if file exists locally
		localStat, localExists := os.Stat(filePath)

		// Handle --new-only flag
		if newOnly && localExists == nil {
			// File exists, check if Drive version is newer
			localModTime := localStat.ModTime()

			// Parse Drive modification time
			driveModTime, err := time.Parse(time.RFC3339, item.ModifiedTime)
			if err == nil {
				// Compare timestamps
				if !driveModTime.After(localModTime) {
					// Drive version is not newer, skip
					color.Cyan("Skipped (not newer): %s", item.Name)
					continue
				}

				// Drive version is newer
				if !overwrite {
					// Ask for confirmation
					if !confirmOverwrite(filePath, item.Size) {
						color.Yellow("Skipped: %s", filePath)
						continue
					}
				}
				// If overwrite is true, will download automatically
			}
		} else if !newOnly && localExists == nil {
			// Standard overwrite check (when --new-only is not used)
			if !overwrite {
				if !confirmOverwrite(filePath, item.Size) {
					color.Yellow("Skipped: %s", filePath)
					continue
				}
			}
		}

		wg.Add(1)
		go func(fileItem *driveapi.File, path string) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			// Download file
			if err := ds.DownloadFile(fileItem.Id, path, true, true); err != nil {
				errMu.Lock()
				errors = append(errors, fmt.Errorf("failed to download %s: %v", fileItem.Name, err))
				errMu.Unlock()
			}
		}(item, filePath)
	}

	// Wait for all downloads to complete
	wg.Wait()

	// Return first error if any occurred
	if len(errors) > 0 {
		return errors[0]
	}

	return nil
}

func runFolderList(cmd *cobra.Command, args []string) error {
	ds, err := getDriveService()
	if err != nil {
		return err
	}

	remoteFolder := args[0]

	// Get folder ID
	var folderID string
	if useIDFlag {
		folderID = remoteFolder
	} else {
		folderID, err = ds.ResolvePath(remoteFolder, true)
		if err != nil {
			return fmt.Errorf("folder not found: %v", err)
		}
	}

	// Get folder contents
	items, err := ds.ListFolder(folderID)
	if err != nil {
		return err
	}

	if len(items) == 0 {
		color.Yellow("Folder '%s' is empty", remoteFolder)
		return nil
	}

	// Sort: folders first, then files
	sort.Slice(items, func(i, j int) bool {
		if ds.IsFolder(items[i]) != ds.IsFolder(items[j]) {
			return ds.IsFolder(items[i])
		}
		return items[i].Name < items[j].Name
	})

	// Print header
	fmt.Printf("\nContents of %s\n", remoteFolder)
	fmt.Println(strings.Repeat("─", 120))
	fmt.Printf("%-10s %-40s %-44s %-20s %12s\n", "Type", "Name", "ID", "Modified", "Size")
	fmt.Println(strings.Repeat("─", 120))

	// Print rows
	for _, item := range items {
		printItemRow(ds, item)
	}

	fmt.Println(strings.Repeat("─", 120))
	fmt.Printf("\nTotal items: %d\n", len(items))

	return nil
}

// Run functions for search and activity commands

func runSearch(cmd *cobra.Command, args []string) error {
	ds, err := getDriveService()
	if err != nil {
		return err
	}

	query := args[0]

	// Parse file types if provided
	var fileTypes []string
	if fileTypeFlag != "" {
		fileTypes = strings.Split(fileTypeFlag, ",")
		for i, ft := range fileTypes {
			fileTypes[i] = strings.TrimSpace(ft)
		}
		color.Cyan("Searching for: %s (types: %s)", query, strings.Join(fileTypes, ", "))
	} else {
		color.Cyan("Searching for: %s", query)
	}

	// Search for files
	items, err := ds.SearchFiles(query, fileTypes, maxResults)
	if err != nil {
		return err
	}

	if len(items) == 0 {
		color.Yellow("No results found")
		return nil
	}

	// Print header
	fmt.Printf("\nSearch Results for '%s'\n", query)
	fmt.Println(strings.Repeat("─", 120))
	fmt.Printf("%-10s %-40s %-44s %-20s %12s\n", "Type", "Name", "ID", "Modified", "Size")
	fmt.Println(strings.Repeat("─", 120))

	// Print rows
	for _, item := range items {
		printItemRow(ds, item)
	}

	fmt.Println(strings.Repeat("─", 120))
	fmt.Printf("\nFound %d items\n", len(items))

	return nil
}

func runActivityChanges(cmd *cobra.Command, args []string) error {
	ds, err := getDriveService()
	if err != nil {
		return err
	}

	// Get changes
	changes, err := ds.ListChanges(maxResults)
	if err != nil {
		return err
	}

	if len(changes) == 0 {
		fmt.Println("No recent changes found")
		return nil
	}

	// Display header
	color.Cyan("\nRecent Changes:")
	fmt.Printf("%-15s %-40s %-30s %-15s\n", "Type", "File Name", "Modified By", "Time")
	fmt.Println(strings.Repeat("-", 100))

	// Display changes
	for _, change := range changes {
		changeType := change.ChangeType
		if change.Removed {
			changeType = color.RedString("Removed")
		} else if change.ChangeType == "Modified" {
			changeType = color.YellowString("Modified")
		} else {
			changeType = color.GreenString("Added")
		}

		fileName := change.FileName
		if fileName == "" {
			fileName = color.New(color.Faint).Sprint("<unnamed>")
		}
		if len(fileName) > 40 {
			fileName = fileName[:37] + "..."
		}

		modifiedBy := change.ModifiedBy
		if modifiedBy == "" {
			modifiedBy = color.New(color.Faint).Sprint("<unknown>")
		}
		if len(modifiedBy) > 30 {
			modifiedBy = modifiedBy[:27] + "..."
		}

		timeStr := change.ChangeTime.Format("2006-01-02 15:04")

		fmt.Printf("%-15s %-40s %-30s %-15s\n", changeType, fileName, modifiedBy, timeStr)
	}

	fmt.Printf("\nTotal: %d changes\n", len(changes))
	return nil
}

func runActivityRevisions(cmd *cobra.Command, args []string) error {
	ds, err := getDriveService()
	if err != nil {
		return err
	}

	filePath := args[0]
	var fileID string

	if useIDFlag {
		fileID = filePath
	} else {
		// Parse path to get folder and filename
		dir := filepath.Dir(filePath)
		filename := filepath.Base(filePath)

		parentID, err := ds.ResolvePath(dir, true)
		if err != nil {
			return fmt.Errorf("parent folder not found: %v", err)
		}

		file, err := ds.FindFile(filename, parentID)
		if err != nil {
			return err
		}
		if file == nil {
			return fmt.Errorf("file not found: %s", filePath)
		}
		fileID = file.Id
	}

	// Get file info
	fileInfo, err := ds.GetFileInfo(fileID)
	if err != nil {
		return fmt.Errorf("unable to get file info: %v", err)
	}

	// Get revisions
	revisions, err := ds.ListRevisions(fileID)
	if err != nil {
		return err
	}

	if len(revisions) == 0 {
		fmt.Println("No revisions found for this file")
		return nil
	}

	// Display file info
	color.Cyan("\nRevision History for: %s", fileInfo.Name)
	fmt.Printf("File ID: %s\n", fileID)
	fmt.Printf("Path: %v\n\n", fileInfo.Path)

	// Display header
	fmt.Printf("%-15s %-25s %-15s %-30s %-10s\n", "Revision ID", "Modified Time", "Size", "Modified By", "Keep")
	fmt.Println(strings.Repeat("-", 100))

	// Display revisions (reverse order - newest first)
	for i := len(revisions) - 1; i >= 0; i-- {
		rev := revisions[i]

		revID := rev.ID
		if len(revID) > 15 {
			revID = revID[:12] + "..."
		}

		modifiedTime := rev.ModifiedTime.Format("2006-01-02 15:04:05")

		sizeStr := formatSize(rev.Size)

		modifiedBy := rev.ModifiedBy
		if modifiedBy == "" {
			modifiedBy = color.New(color.Faint).Sprint("<unknown>")
		}
		if len(modifiedBy) > 30 {
			modifiedBy = modifiedBy[:27] + "..."
		}

		keepStr := ""
		if rev.KeepForever {
			keepStr = color.GreenString("Yes")
		}

		fmt.Printf("%-15s %-25s %-15s %-30s %-10s\n", revID, modifiedTime, sizeStr, modifiedBy, keepStr)
	}

	fmt.Printf("\nTotal: %d revisions\n", len(revisions))
	if len(revisions) >= 100 {
		color.Yellow("\nNote: For frequently edited files, older revisions might be omitted from the list.")
	}
	return nil
}

func runActivityDeleted(cmd *cobra.Command, args []string) error {
	ds, err := getDriveService()
	if err != nil {
		return err
	}

	// Get deleted files
	files, err := ds.ListTrashedFiles(daysBackFlag, maxResults)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		fmt.Printf("No deleted files found in the last %d days\n", daysBackFlag)
		return nil
	}

	// Display header
	color.Cyan("\nDeleted Files (Last %d days):", daysBackFlag)
	fmt.Printf("%-40s %-25s %-15s %-30s\n", "File Name", "Deleted Time", "Size", "Deleted By")
	fmt.Println(strings.Repeat("-", 110))

	// Display deleted files
	for _, file := range files {
		fileName := file.Name
		if fileName == "" {
			fileName = color.New(color.Faint).Sprint("<unnamed>")
		}
		if len(fileName) > 40 {
			fileName = fileName[:37] + "..."
		}

		deletedTime := ""
		if file.TrashedTime != "" {
			if t, err := time.Parse(time.RFC3339, file.TrashedTime); err == nil {
				deletedTime = t.Format("2006-01-02 15:04:05")
			}
		}

		sizeStr := formatSize(file.Size)

		deletedBy := ""
		if file.TrashingUser != nil {
			deletedBy = file.TrashingUser.DisplayName
			if deletedBy == "" {
				deletedBy = file.TrashingUser.EmailAddress
			}
		}
		if deletedBy == "" {
			deletedBy = color.New(color.Faint).Sprint("<unknown>")
		}
		if len(deletedBy) > 30 {
			deletedBy = deletedBy[:27] + "..."
		}

		fmt.Printf("%-40s %-25s %-15s %-30s\n", fileName, deletedTime, sizeStr, deletedBy)
	}

	fmt.Printf("\nTotal: %d deleted files\n", len(files))
	return nil
}

func runActivityHistory(cmd *cobra.Command, args []string) error {
	// Get Activity service
	activityService, err := auth.GetAuthenticatedActivityService(globalConfig)
	if err != nil {
		return err
	}

	// Query activities
	activities, err := drive.QueryDriveActivity(activityService, daysBackFlag, maxResults)
	if err != nil {
		return err
	}

	if len(activities) == 0 {
		fmt.Printf("No activities found in the last %d days\n", daysBackFlag)
		return nil
	}

	// Display header
	color.Cyan("\nActivity History (Last %d days):", daysBackFlag)
	fmt.Printf("%-20s %-15s %-30s %-40s %-30s\n", "Time", "Action", "Detail", "File/Item", "Actor")
	fmt.Println(strings.Repeat("-", 135))

	// Display activities
	for _, activity := range activities {
		timestamp := activity.Timestamp.Format("2006-01-02 15:04:05")

		actionType := activity.ActionType
		// Color code based on action type
		switch activity.ActionType {
		case "Delete":
			actionType = color.RedString(activity.ActionType)
		case "Create", "Upload":
			actionType = color.GreenString(activity.ActionType)
		case "Edit":
			actionType = color.YellowString(activity.ActionType)
		case "Permission":
			actionType = color.CyanString(activity.ActionType)
		}

		actionDetail := activity.ActionDetail
		if len(actionDetail) > 30 {
			actionDetail = actionDetail[:27] + "..."
		}

		targetTitle := "<no target>"
		if len(activity.TargetTitles) > 0 {
			targetTitle = activity.TargetTitles[0]
		}
		if len(targetTitle) > 40 {
			targetTitle = targetTitle[:37] + "..."
		}

		actor := "<unknown>"
		if len(activity.Actors) > 0 {
			actor = activity.Actors[0]
		}
		if len(actor) > 30 {
			actor = actor[:27] + "..."
		}

		fmt.Printf("%-20s %-15s %-30s %-40s %-30s\n", timestamp, actionType, actionDetail, targetTitle, actor)
	}

	fmt.Printf("\nTotal: %d activities\n", len(activities))
	color.Yellow("\n⚠  Note: Google Drive API doesn't retain file names for permanently deleted files.")
	color.Yellow("   File names show as <ID: ...> for permanent deletions.")
	color.Yellow("   Use 'gdrive activity deleted' to see files still in trash with their names.")
	return nil
}
