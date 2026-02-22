package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gdrive/internal/auth"
	"gdrive/internal/drive"

	"github.com/mark3labs/mcp-go/mcp"
	driveapi "google.golang.org/api/drive/v3"
	"google.golang.org/api/driveactivity/v2"
)

// RegisterReadTools registers all read-only MCP tools on the server.
func RegisterReadTools(s *Server) {
	registerSearchTool(s)
	registerFolderListTool(s)
	registerFileInfoTool(s)
	registerDownloadURLTool(s)
	registerExportURLTool(s)
	registerActivityChangesTool(s)
	registerActivityDeletedTool(s)
	registerActivityHistoryTool(s)
	registerFileRevisionsTool(s)
}

// RegisterWriteTools registers all write MCP tools on the server.
func RegisterWriteTools(s *Server) {
	registerDeleteTool(s)
	registerRenameTool(s)
	registerMoveTool(s)
	registerCopyTool(s)
	registerFolderCreateTool(s)
	registerPermissionsListTool(s)
	registerPermissionsUpdateTool(s)
	registerCreateUploadURLTool(s)
}

// getDriveService creates an authenticated Drive service from context.
func getDriveService(ctx context.Context) (*drive.Service, error) {
	cfg := auth.NewConfig("", "")
	srv, err := auth.GetAuthenticatedServiceWithContext(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}
	return drive.NewService(srv), nil
}

// getActivityService creates an authenticated Drive Activity service from context.
func getActivityService(ctx context.Context) (*driveactivity.Service, error) {
	cfg := auth.NewConfig("", "")
	srv, err := auth.GetAuthenticatedActivityServiceWithContext(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}
	return srv, nil
}

// toolResult returns a JSON-formatted MCP tool result.
func toolResult(data interface{}) (*mcp.CallToolResult, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}
	return mcp.NewToolResultText(string(b)), nil
}

// logToolCall logs a tool invocation and returns the result.
func logToolCall(toolName string, start time.Time, result *mcp.CallToolResult, err error) (*mcp.CallToolResult, error) {
	duration := time.Since(start)
	if err != nil {
		slog.Error("tool call failed", "tool", toolName, "duration", duration, "error", err)
	} else {
		slog.Info("tool call", "tool", toolName, "duration", duration)
	}
	return result, err
}

// --- Read Tools ---

func registerSearchTool(s *Server) {
	tool := mcp.NewTool("drive_search",
		mcp.WithDescription("Search for files and folders in Google Drive by name. Use type shortcuts (image, audio, video, prez, doc, spreadsheet, txt, pdf, folder) or explicit MIME types to filter results."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search query (file name)")),
		mcp.WithString("fileTypes", mcp.Description("Comma-separated file type shortcuts or MIME types (e.g., 'image,pdf' or 'application/pdf')")),
		mcp.WithNumber("maxResults", mcp.Description("Maximum number of results (default: 50)")),
	)

	s.mcpServer.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()

		query, _ := req.GetArguments()["query"].(string)
		fileTypesStr, _ := req.GetArguments()["fileTypes"].(string)
		maxResults := int64(50)
		if mr, ok := req.GetArguments()["maxResults"].(float64); ok && mr > 0 {
			maxResults = int64(mr)
		}

		driveSrv, err := getDriveService(ctx)
		if err != nil {
			return logToolCall("drive_search", start, nil, err)
		}

		var fileTypes []string
		if fileTypesStr != "" {
			fileTypes = strings.Split(fileTypesStr, ",")
			for i := range fileTypes {
				fileTypes[i] = strings.TrimSpace(fileTypes[i])
			}
		}

		files, err := driveSrv.SearchFiles(query, fileTypes, maxResults)
		if err != nil {
			return logToolCall("drive_search", start, nil, fmt.Errorf("search failed: %w", err))
		}

		results := make([]map[string]interface{}, 0, len(files))
		for _, f := range files {
			results = append(results, map[string]interface{}{
				"id":           f.Id,
				"name":         f.Name,
				"mimeType":     f.MimeType,
				"modifiedTime": f.ModifiedTime,
				"size":         f.Size,
			})
		}

		result, err := toolResult(results)
		return logToolCall("drive_search", start, result, err)
	})
}

func registerFolderListTool(s *Server) {
	tool := mcp.NewTool("drive_folder_list",
		mcp.WithDescription("List contents of a Google Drive folder. Returns files and subfolders sorted by type (folders first) then alphabetically."),
		mcp.WithString("folderId", mcp.Required(), mcp.Description("Google Drive folder ID (use 'root' for My Drive root)")),
	)

	s.mcpServer.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()

		folderID, _ := req.GetArguments()["folderId"].(string)

		driveSrv, err := getDriveService(ctx)
		if err != nil {
			return logToolCall("drive_folder_list", start, nil, err)
		}

		files, err := driveSrv.ListFolder(folderID)
		if err != nil {
			return logToolCall("drive_folder_list", start, nil, fmt.Errorf("list folder failed: %w", err))
		}

		results := make([]map[string]interface{}, 0, len(files))
		for _, f := range files {
			results = append(results, map[string]interface{}{
				"id":           f.Id,
				"name":         f.Name,
				"mimeType":     f.MimeType,
				"modifiedTime": f.ModifiedTime,
				"size":         f.Size,
			})
		}

		result, err := toolResult(results)
		return logToolCall("drive_folder_list", start, result, err)
	})
}

func registerFileInfoTool(s *Server) {
	tool := mcp.NewTool("drive_file_info",
		mcp.WithDescription("Get detailed metadata for a Google Drive file including full path from root, owners, timestamps, and web link."),
		mcp.WithString("fileId", mcp.Required(), mcp.Description("Google Drive file ID")),
	)

	s.mcpServer.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()

		fileID, _ := req.GetArguments()["fileId"].(string)

		driveSrv, err := getDriveService(ctx)
		if err != nil {
			return logToolCall("drive_file_info", start, nil, err)
		}

		info, err := driveSrv.GetFileInfo(fileID)
		if err != nil {
			return logToolCall("drive_file_info", start, nil, fmt.Errorf("get file info failed: %w", err))
		}

		// Build path string
		pathParts := make([]string, 0, len(info.Path))
		for _, p := range info.Path {
			pathParts = append(pathParts, p.Name)
		}

		// Build owners list
		owners := make([]map[string]string, 0)
		for _, o := range info.Owners {
			owners = append(owners, map[string]string{
				"displayName":  o.DisplayName,
				"emailAddress": o.EmailAddress,
			})
		}

		data := map[string]interface{}{
			"id":           info.ID,
			"name":         info.Name,
			"mimeType":     info.MimeType,
			"size":         info.Size,
			"createdTime":  info.CreatedTime,
			"modifiedTime": info.ModifiedTime,
			"webViewLink":  info.WebViewLink,
			"owners":       owners,
			"path":         pathParts,
		}

		result, err := toolResult(data)
		return logToolCall("drive_file_info", start, result, err)
	})
}

func registerDownloadURLTool(s *Server) {
	tool := mcp.NewTool("drive_download_url",
		mcp.WithDescription("Get an authenticated download URL for a Google Drive file. For Google Workspace files (Docs, Sheets, Slides), use drive_export_url instead."),
		mcp.WithString("fileId", mcp.Required(), mcp.Description("Google Drive file ID")),
	)

	s.mcpServer.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()

		fileID, _ := req.GetArguments()["fileId"].(string)

		driveSrv, err := getDriveService(ctx)
		if err != nil {
			return logToolCall("drive_download_url", start, nil, err)
		}

		// Get file metadata
		file, err := driveSrv.API.Files.Get(fileID).
			Fields("id, name, mimeType, size").Do()
		if err != nil {
			return logToolCall("drive_download_url", start, nil, fmt.Errorf("file not found: %w", err))
		}

		// Reject Google Workspace files
		if driveSrv.IsGoogleWorkspaceFile(file) {
			return logToolCall("drive_download_url", start, nil,
				fmt.Errorf("cannot download Google Workspace file '%s' (%s). Use drive_export_url to export it to a standard format", file.Name, file.MimeType))
		}

		// Get access token from context for URL
		token, ok := auth.GetAccessTokenFromContext(ctx)
		if !ok {
			return logToolCall("drive_download_url", start, nil, fmt.Errorf("no access token in context"))
		}

		downloadURL := fmt.Sprintf("https://www.googleapis.com/drive/v3/files/%s?alt=media&access_token=%s", fileID, token.AccessToken)

		data := map[string]interface{}{
			"downloadUrl": downloadURL,
			"fileName":    file.Name,
			"mimeType":    file.MimeType,
			"size":        file.Size,
			"expiresIn":   3600,
		}

		result, err := toolResult(data)
		return logToolCall("drive_download_url", start, result, err)
	})
}

func registerExportURLTool(s *Server) {
	tool := mcp.NewTool("drive_export_url",
		mcp.WithDescription("Get an authenticated export URL for Google Workspace files (Docs, Sheets, Slides). Converts to standard formats like PDF, DOCX, XLSX, PPTX, CSV, TXT, HTML."),
		mcp.WithString("fileId", mcp.Required(), mcp.Description("Google Drive file ID (must be a Google Workspace file)")),
		mcp.WithString("format", mcp.Required(), mcp.Description("Export format: pdf, docx, xlsx, pptx, csv, txt, html")),
	)

	s.mcpServer.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()

		fileID, _ := req.GetArguments()["fileId"].(string)
		format, _ := req.GetArguments()["format"].(string)

		driveSrv, err := getDriveService(ctx)
		if err != nil {
			return logToolCall("drive_export_url", start, nil, err)
		}

		// Get file metadata
		file, err := driveSrv.API.Files.Get(fileID).
			Fields("id, name, mimeType").Do()
		if err != nil {
			return logToolCall("drive_export_url", start, nil, fmt.Errorf("file not found: %w", err))
		}

		// Reject non-Workspace files
		if !driveSrv.IsGoogleWorkspaceFile(file) {
			return logToolCall("drive_export_url", start, nil,
				fmt.Errorf("file '%s' (%s) is not a Google Workspace file. Use drive_download_url instead", file.Name, file.MimeType))
		}

		// Validate export format
		exportMimeType := driveSrv.GetExportMimeType(file.MimeType, format)
		if exportMimeType == "" {
			supportedFormats := []string{}
			if formats, ok := drive.ExportFormats[file.MimeType]; ok {
				for f := range formats {
					supportedFormats = append(supportedFormats, f)
				}
			}
			return logToolCall("drive_export_url", start, nil,
				fmt.Errorf("unsupported export format '%s' for %s. Supported formats: %s", format, file.MimeType, strings.Join(supportedFormats, ", ")))
		}

		// Get access token from context for URL
		token, ok := auth.GetAccessTokenFromContext(ctx)
		if !ok {
			return logToolCall("drive_export_url", start, nil, fmt.Errorf("no access token in context"))
		}

		exportURL := fmt.Sprintf("https://www.googleapis.com/drive/v3/files/%s/export?mimeType=%s&access_token=%s",
			fileID, exportMimeType, token.AccessToken)

		exportedName := driveSrv.AdjustFilename(file.Name, format)

		data := map[string]interface{}{
			"exportUrl":      exportURL,
			"fileName":       exportedName,
			"exportMimeType": exportMimeType,
			"expiresIn":      3600,
		}

		result, err := toolResult(data)
		return logToolCall("drive_export_url", start, result, err)
	})
}

func registerActivityChangesTool(s *Server) {
	tool := mcp.NewTool("drive_activity_changes",
		mcp.WithDescription("List recent changes to files in your Google Drive. Shows what files were added, modified, or removed."),
		mcp.WithNumber("maxResults", mcp.Description("Maximum number of results (default: 50)")),
	)

	s.mcpServer.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()

		maxResults := int64(50)
		if mr, ok := req.GetArguments()["maxResults"].(float64); ok && mr > 0 {
			maxResults = int64(mr)
		}

		driveSrv, err := getDriveService(ctx)
		if err != nil {
			return logToolCall("drive_activity_changes", start, nil, err)
		}

		changes, err := driveSrv.ListChanges(maxResults)
		if err != nil {
			return logToolCall("drive_activity_changes", start, nil, fmt.Errorf("list changes failed: %w", err))
		}

		results := make([]map[string]interface{}, 0, len(changes))
		for _, c := range changes {
			results = append(results, map[string]interface{}{
				"fileId":     c.FileID,
				"fileName":   c.FileName,
				"changeType": c.ChangeType,
				"changeTime": c.ChangeTime.Format(time.RFC3339),
				"modifiedBy": c.ModifiedBy,
			})
		}

		result, err := toolResult(results)
		return logToolCall("drive_activity_changes", start, result, err)
	})
}

func registerActivityDeletedTool(s *Server) {
	tool := mcp.NewTool("drive_activity_deleted",
		mcp.WithDescription("List recently deleted (trashed) files in Google Drive within a time window."),
		mcp.WithNumber("daysBack", mcp.Description("Number of days to look back (default: 7)")),
		mcp.WithNumber("maxResults", mcp.Description("Maximum number of results (default: 100)")),
	)

	s.mcpServer.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()

		daysBack := 7
		if db, ok := req.GetArguments()["daysBack"].(float64); ok && db > 0 {
			daysBack = int(db)
		}
		maxResults := int64(100)
		if mr, ok := req.GetArguments()["maxResults"].(float64); ok && mr > 0 {
			maxResults = int64(mr)
		}

		driveSrv, err := getDriveService(ctx)
		if err != nil {
			return logToolCall("drive_activity_deleted", start, nil, err)
		}

		files, err := driveSrv.ListTrashedFiles(daysBack, maxResults)
		if err != nil {
			return logToolCall("drive_activity_deleted", start, nil, fmt.Errorf("list trashed files failed: %w", err))
		}

		results := make([]map[string]interface{}, 0, len(files))
		for _, f := range files {
			entry := map[string]interface{}{
				"id":          f.Id,
				"name":        f.Name,
				"trashedTime": f.TrashedTime,
				"size":        f.Size,
			}
			if f.TrashingUser != nil {
				entry["trashedBy"] = f.TrashingUser.DisplayName
				if entry["trashedBy"] == "" {
					entry["trashedBy"] = f.TrashingUser.EmailAddress
				}
			}
			results = append(results, entry)
		}

		result, err := toolResult(results)
		return logToolCall("drive_activity_deleted", start, result, err)
	})
}

func registerActivityHistoryTool(s *Server) {
	tool := mcp.NewTool("drive_activity_history",
		mcp.WithDescription("Query comprehensive activity history from Google Drive Activity API. Includes edits, moves, permission changes, deletions, and more. Hard cap of 200 results."),
		mcp.WithNumber("daysBack", mcp.Description("Number of days to look back (default: 7)")),
		mcp.WithNumber("maxResults", mcp.Description("Maximum number of results (default: 100, hard cap: 200)")),
	)

	s.mcpServer.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()

		daysBack := 7
		if db, ok := req.GetArguments()["daysBack"].(float64); ok && db > 0 {
			daysBack = int(db)
		}
		maxResults := int64(100)
		if mr, ok := req.GetArguments()["maxResults"].(float64); ok && mr > 0 {
			maxResults = int64(mr)
		}
		// Hard cap at 200
		if maxResults > 200 {
			maxResults = 200
		}

		activitySrv, err := getActivityService(ctx)
		if err != nil {
			return logToolCall("drive_activity_history", start, nil, err)
		}

		activities, err := drive.QueryDriveActivity(activitySrv, daysBack, maxResults)
		if err != nil {
			return logToolCall("drive_activity_history", start, nil, fmt.Errorf("query activity failed: %w", err))
		}

		results := make([]map[string]interface{}, 0, len(activities))
		for _, a := range activities {
			results = append(results, map[string]interface{}{
				"timestamp":    a.Timestamp.Format(time.RFC3339),
				"actionType":   a.ActionType,
				"actionDetail": a.ActionDetail,
				"actors":       a.Actors,
				"targets":      a.Targets,
				"targetTitles": a.TargetTitles,
			})
		}

		result, err := toolResult(results)
		return logToolCall("drive_activity_history", start, result, err)
	})
}

func registerFileRevisionsTool(s *Server) {
	tool := mcp.NewTool("drive_file_revisions",
		mcp.WithDescription("List revision history for a specific Google Drive file. Shows version history with modification times, authors, and sizes."),
		mcp.WithString("fileId", mcp.Required(), mcp.Description("Google Drive file ID")),
	)

	s.mcpServer.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()

		fileID, _ := req.GetArguments()["fileId"].(string)

		driveSrv, err := getDriveService(ctx)
		if err != nil {
			return logToolCall("drive_file_revisions", start, nil, err)
		}

		revisions, err := driveSrv.ListRevisions(fileID)
		if err != nil {
			return logToolCall("drive_file_revisions", start, nil, fmt.Errorf("list revisions failed: %w", err))
		}

		results := make([]map[string]interface{}, 0, len(revisions))
		for _, r := range revisions {
			results = append(results, map[string]interface{}{
				"id":           r.ID,
				"modifiedTime": r.ModifiedTime.Format(time.RFC3339),
				"size":         r.Size,
				"modifiedBy":   r.ModifiedBy,
				"keepForever":  r.KeepForever,
			})
		}

		result, err := toolResult(results)
		return logToolCall("drive_file_revisions", start, result, err)
	})
}

// --- Write Tools ---

func registerDeleteTool(s *Server) {
	tool := mcp.NewTool("drive_delete",
		mcp.WithDescription("Move a file or folder to trash in Google Drive. This is a soft delete - files can be recovered from trash."),
		mcp.WithString("fileId", mcp.Required(), mcp.Description("Google Drive file or folder ID to trash")),
	)

	s.mcpServer.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()

		fileID, _ := req.GetArguments()["fileId"].(string)

		driveSrv, err := getDriveService(ctx)
		if err != nil {
			return logToolCall("drive_delete", start, nil, err)
		}

		// Get file info first for the response
		file, err := driveSrv.API.Files.Get(fileID).Fields("id, name, trashed").Do()
		if err != nil {
			return logToolCall("drive_delete", start, nil, fmt.Errorf("file not found: %w", err))
		}

		if file.Trashed {
			data := map[string]interface{}{
				"fileId":   file.Id,
				"fileName": file.Name,
				"message":  "File is already in trash",
			}
			result, err := toolResult(data)
			return logToolCall("drive_delete", start, result, err)
		}

		// Soft delete: set trashed = true
		_, err = driveSrv.API.Files.Update(fileID, &driveapi.File{Trashed: true}).Do()
		if err != nil {
			return logToolCall("drive_delete", start, nil, fmt.Errorf("trash file failed: %w", err))
		}

		data := map[string]interface{}{
			"fileId":   file.Id,
			"fileName": file.Name,
			"message":  "File moved to trash",
		}

		result, err := toolResult(data)
		return logToolCall("drive_delete", start, result, err)
	})
}

func registerRenameTool(s *Server) {
	tool := mcp.NewTool("drive_rename",
		mcp.WithDescription("Rename a file or folder in Google Drive."),
		mcp.WithString("fileId", mcp.Required(), mcp.Description("Google Drive file or folder ID")),
		mcp.WithString("newName", mcp.Required(), mcp.Description("New name for the file or folder")),
	)

	s.mcpServer.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()

		fileID, _ := req.GetArguments()["fileId"].(string)
		newName, _ := req.GetArguments()["newName"].(string)

		if newName == "" {
			return logToolCall("drive_rename", start, nil, fmt.Errorf("newName is required"))
		}

		driveSrv, err := getDriveService(ctx)
		if err != nil {
			return logToolCall("drive_rename", start, nil, err)
		}

		file, err := driveSrv.RenameFile(fileID, newName)
		if err != nil {
			return logToolCall("drive_rename", start, nil, fmt.Errorf("rename failed: %w", err))
		}

		data := map[string]interface{}{
			"id":          file.Id,
			"name":        file.Name,
			"webViewLink": file.WebViewLink,
		}

		result, err := toolResult(data)
		return logToolCall("drive_rename", start, result, err)
	})
}

func registerMoveTool(s *Server) {
	tool := mcp.NewTool("drive_move",
		mcp.WithDescription("Move a file or folder to a different folder in Google Drive."),
		mcp.WithString("fileId", mcp.Required(), mcp.Description("Google Drive file or folder ID to move")),
		mcp.WithString("targetFolderId", mcp.Required(), mcp.Description("ID of the destination folder")),
	)

	s.mcpServer.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()

		fileID, _ := req.GetArguments()["fileId"].(string)
		targetFolderID, _ := req.GetArguments()["targetFolderId"].(string)

		driveSrv, err := getDriveService(ctx)
		if err != nil {
			return logToolCall("drive_move", start, nil, err)
		}

		file, err := driveSrv.MoveFile(fileID, targetFolderID)
		if err != nil {
			return logToolCall("drive_move", start, nil, fmt.Errorf("move failed: %w", err))
		}

		data := map[string]interface{}{
			"id":   file.Id,
			"name": file.Name,
		}

		result, err := toolResult(data)
		return logToolCall("drive_move", start, result, err)
	})
}

func registerCopyTool(s *Server) {
	tool := mcp.NewTool("drive_copy",
		mcp.WithDescription("Copy a file in Google Drive to a target folder with an optional new name."),
		mcp.WithString("fileId", mcp.Required(), mcp.Description("Google Drive file ID to copy")),
		mcp.WithString("targetFolderId", mcp.Required(), mcp.Description("ID of the destination folder")),
		mcp.WithString("newName", mcp.Description("Optional new name for the copy")),
	)

	s.mcpServer.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()

		fileID, _ := req.GetArguments()["fileId"].(string)
		targetFolderID, _ := req.GetArguments()["targetFolderId"].(string)
		newName, _ := req.GetArguments()["newName"].(string)

		driveSrv, err := getDriveService(ctx)
		if err != nil {
			return logToolCall("drive_copy", start, nil, err)
		}

		file, err := driveSrv.CopyFile(fileID, drive.CopyOptions{
			NewName:        newName,
			ParentFolderID: targetFolderID,
		})
		if err != nil {
			return logToolCall("drive_copy", start, nil, fmt.Errorf("copy failed: %w", err))
		}

		data := map[string]interface{}{
			"id":          file.Id,
			"name":        file.Name,
			"webViewLink": file.WebViewLink,
		}

		result, err := toolResult(data)
		return logToolCall("drive_copy", start, result, err)
	})
}

func registerFolderCreateTool(s *Server) {
	tool := mcp.NewTool("drive_folder_create",
		mcp.WithDescription("Create a new folder in Google Drive under a specified parent folder."),
		mcp.WithString("parentFolderId", mcp.Required(), mcp.Description("ID of the parent folder (use 'root' for My Drive root)")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Name of the new folder")),
	)

	s.mcpServer.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()

		parentFolderID, _ := req.GetArguments()["parentFolderId"].(string)
		name, _ := req.GetArguments()["name"].(string)

		if name == "" {
			return logToolCall("drive_folder_create", start, nil, fmt.Errorf("name is required"))
		}

		driveSrv, err := getDriveService(ctx)
		if err != nil {
			return logToolCall("drive_folder_create", start, nil, err)
		}

		folder := &driveapi.File{
			Name:     name,
			MimeType: drive.DriveFolderMimeType,
			Parents:  []string{parentFolderID},
		}

		created, err := driveSrv.API.Files.Create(folder).
			Fields("id, name, mimeType, webViewLink").Do()
		if err != nil {
			return logToolCall("drive_folder_create", start, nil, fmt.Errorf("create folder failed: %w", err))
		}

		data := map[string]interface{}{
			"id":          created.Id,
			"name":        created.Name,
			"mimeType":    created.MimeType,
			"webViewLink": created.WebViewLink,
		}

		result, err := toolResult(data)
		return logToolCall("drive_folder_create", start, result, err)
	})
}

func registerPermissionsListTool(s *Server) {
	tool := mcp.NewTool("drive_permissions_list",
		mcp.WithDescription("List all permissions (sharing settings) for a Google Drive file or folder."),
		mcp.WithString("fileId", mcp.Required(), mcp.Description("Google Drive file or folder ID")),
	)

	s.mcpServer.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()

		fileID, _ := req.GetArguments()["fileId"].(string)

		driveSrv, err := getDriveService(ctx)
		if err != nil {
			return logToolCall("drive_permissions_list", start, nil, err)
		}

		perms, err := driveSrv.ListPermissions(fileID)
		if err != nil {
			return logToolCall("drive_permissions_list", start, nil, fmt.Errorf("list permissions failed: %w", err))
		}

		results := make([]map[string]interface{}, 0, len(perms))
		for _, p := range perms {
			results = append(results, map[string]interface{}{
				"id":           p.Id,
				"type":         p.Type,
				"role":         p.Role,
				"emailAddress": p.EmailAddress,
				"displayName":  p.DisplayName,
				"domain":       p.Domain,
			})
		}

		result, err := toolResult(results)
		return logToolCall("drive_permissions_list", start, result, err)
	})
}

func registerPermissionsUpdateTool(s *Server) {
	tool := mcp.NewTool("drive_permissions_update",
		mcp.WithDescription("Add or remove permissions on a Google Drive file. For adding: specify type (user/anyone), role (reader/writer/commenter), and email (if type=user). For removing: specify permissionId."),
		mcp.WithString("fileId", mcp.Required(), mcp.Description("Google Drive file or folder ID")),
		mcp.WithString("action", mcp.Required(), mcp.Description("Action to perform: 'add' or 'remove'")),
		mcp.WithString("type", mcp.Description("Permission type for add: 'user' or 'anyone'")),
		mcp.WithString("role", mcp.Description("Permission role for add: 'reader', 'writer', or 'commenter'")),
		mcp.WithString("email", mcp.Description("Email address (required when type='user')")),
		mcp.WithBoolean("notify", mcp.Description("Send notification email (default: true, only for add+user)")),
		mcp.WithString("message", mcp.Description("Custom message for notification email")),
		mcp.WithString("permissionId", mcp.Description("Permission ID to remove (required for 'remove' action)")),
	)

	s.mcpServer.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()

		fileID, _ := req.GetArguments()["fileId"].(string)
		action, _ := req.GetArguments()["action"].(string)

		driveSrv, err := getDriveService(ctx)
		if err != nil {
			return logToolCall("drive_permissions_update", start, nil, err)
		}

		switch action {
		case "add":
			permType, _ := req.GetArguments()["type"].(string)
			role, _ := req.GetArguments()["role"].(string)
			email, _ := req.GetArguments()["email"].(string)
			message, _ := req.GetArguments()["message"].(string)
			notify := true
			if n, ok := req.GetArguments()["notify"].(bool); ok {
				notify = n
			}

			if permType == "user" {
				if email == "" {
					return logToolCall("drive_permissions_update", start, nil, fmt.Errorf("email is required when type is 'user'"))
				}
				err = driveSrv.ShareFile(fileID, drive.ShareOptions{
					Email:   email,
					Role:    role,
					Notify:  notify,
					Message: message,
				})
			} else if permType == "anyone" {
				err = driveSrv.ShareWithAnyone(fileID, role)
			} else {
				return logToolCall("drive_permissions_update", start, nil, fmt.Errorf("type must be 'user' or 'anyone'"))
			}

			if err != nil {
				return logToolCall("drive_permissions_update", start, nil, fmt.Errorf("add permission failed: %w", err))
			}

		case "remove":
			permissionID, _ := req.GetArguments()["permissionId"].(string)
			if permissionID == "" {
				return logToolCall("drive_permissions_update", start, nil, fmt.Errorf("permissionId is required for remove action"))
			}
			err = driveSrv.RemovePermission(fileID, permissionID)
			if err != nil {
				return logToolCall("drive_permissions_update", start, nil, fmt.Errorf("remove permission failed: %w", err))
			}

		default:
			return logToolCall("drive_permissions_update", start, nil, fmt.Errorf("action must be 'add' or 'remove'"))
		}

		// Return updated permissions list
		perms, err := driveSrv.ListPermissions(fileID)
		if err != nil {
			return logToolCall("drive_permissions_update", start, nil, fmt.Errorf("list permissions failed: %w", err))
		}

		results := make([]map[string]interface{}, 0, len(perms))
		for _, p := range perms {
			results = append(results, map[string]interface{}{
				"id":           p.Id,
				"type":         p.Type,
				"role":         p.Role,
				"emailAddress": p.EmailAddress,
				"displayName":  p.DisplayName,
				"domain":       p.Domain,
			})
		}

		result, err := toolResult(results)
		return logToolCall("drive_permissions_update", start, result, err)
	})
}

func registerCreateUploadURLTool(s *Server) {
	tool := mcp.NewTool("drive_create_upload_url",
		mcp.WithDescription("Create a resumable upload URL for uploading a file to Google Drive. If a file with the same name exists in the target folder, it will create a new version (update)."),
		mcp.WithString("fileName", mcp.Required(), mcp.Description("Name of the file to upload")),
		mcp.WithString("folderId", mcp.Required(), mcp.Description("ID of the target folder")),
		mcp.WithString("mimeType", mcp.Description("MIME type of the file (auto-detected from extension if not provided)")),
	)

	s.mcpServer.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()

		fileName, _ := req.GetArguments()["fileName"].(string)
		folderID, _ := req.GetArguments()["folderId"].(string)
		mimeType, _ := req.GetArguments()["mimeType"].(string)

		driveSrv, err := getDriveService(ctx)
		if err != nil {
			return logToolCall("drive_create_upload_url", start, nil, err)
		}

		// Auto-detect MIME type from extension if not provided
		if mimeType == "" {
			mimeType = detectMimeType(fileName)
		}

		// Check if file already exists (for versioning)
		existing, _ := driveSrv.FindFile(fileName, folderID)
		isUpdate := existing != nil

		// Get access token
		token, ok := auth.GetAccessTokenFromContext(ctx)
		if !ok {
			return logToolCall("drive_create_upload_url", start, nil, fmt.Errorf("no access token in context"))
		}

		var uploadURL string
		var fileID string

		if isUpdate {
			// Update existing file (new version)
			fileID = existing.Id
			uploadURL = fmt.Sprintf("https://www.googleapis.com/upload/drive/v3/files/%s?uploadType=resumable&access_token=%s", fileID, token.AccessToken)
		} else {
			// Create new file
			uploadURL = fmt.Sprintf("https://www.googleapis.com/upload/drive/v3/files?uploadType=resumable&access_token=%s", token.AccessToken)
		}

		data := map[string]interface{}{
			"uploadUrl":        uploadURL,
			"fileId":           fileID,
			"isUpdate":         isUpdate,
			"detectedMimeType": mimeType,
		}

		result, err := toolResult(data)
		return logToolCall("drive_create_upload_url", start, result, err)
	})
}

// detectMimeType returns a MIME type based on file extension.
func detectMimeType(filename string) string {
	ext := strings.ToLower(filename)
	if idx := strings.LastIndex(ext, "."); idx >= 0 {
		ext = ext[idx:]
	}

	mimeTypes := map[string]string{
		".pdf":  "application/pdf",
		".doc":  "application/msword",
		".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		".xls":  "application/vnd.ms-excel",
		".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		".ppt":  "application/vnd.ms-powerpoint",
		".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		".txt":  "text/plain",
		".csv":  "text/csv",
		".html": "text/html",
		".json": "application/json",
		".xml":  "application/xml",
		".zip":  "application/zip",
		".png":  "image/png",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".gif":  "image/gif",
		".svg":  "image/svg+xml",
		".mp4":  "video/mp4",
		".mp3":  "audio/mpeg",
		".wav":  "audio/wav",
	}

	if mt, ok := mimeTypes[ext]; ok {
		return mt
	}
	return "application/octet-stream"
}
