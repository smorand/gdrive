package mcp

import (
	"testing"
)

// --- Ping ---

func TestPingTool(t *testing.T) {
	srv := setupToolTest(t)

	result, err := callTool(t, srv, "ping", nil)
	if err != nil {
		t.Fatalf("ping failed: %v", err)
	}

	data := extractResultJSON(t, result)
	if data["message"] != "pong" {
		t.Errorf("expected message=pong, got %v", data["message"])
	}
}

// --- drive_search ---

func TestDriveSearch(t *testing.T) {
	srv := setupToolTest(t)

	t.Run("basic search", func(t *testing.T) {
		result, err := callTool(t, srv, "drive_search", map[string]interface{}{
			"query": "Test",
		})
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}

		data := extractResultArray(t, result)
		if len(data) == 0 {
			t.Error("expected results, got empty")
		}
	})

	t.Run("no results", func(t *testing.T) {
		result, err := callTool(t, srv, "drive_search", map[string]interface{}{
			"query": "nonexistent-file-xyz",
		})
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}

		data := extractResultArray(t, result)
		if len(data) != 0 {
			t.Errorf("expected no results, got %d", len(data))
		}
	})
}

// --- drive_folder_list ---

func TestDriveFolderList(t *testing.T) {
	srv := setupToolTest(t)

	t.Run("list folder contents", func(t *testing.T) {
		result, err := callTool(t, srv, "drive_folder_list", map[string]interface{}{
			"folderId": "folder-1",
		})
		if err != nil {
			t.Fatalf("folder list failed: %v", err)
		}

		data := extractResultArray(t, result)
		if len(data) == 0 {
			t.Error("expected files in folder, got empty")
		}
	})

	t.Run("list empty folder", func(t *testing.T) {
		result, err := callTool(t, srv, "drive_folder_list", map[string]interface{}{
			"folderId": "empty-folder",
		})
		if err != nil {
			t.Fatalf("folder list failed: %v", err)
		}

		data := extractResultArray(t, result)
		if len(data) != 0 {
			t.Errorf("expected empty, got %d items", len(data))
		}
	})
}

// --- drive_file_info ---

func TestDriveFileInfo(t *testing.T) {
	srv := setupToolTest(t)

	t.Run("regular file", func(t *testing.T) {
		result, err := callTool(t, srv, "drive_file_info", map[string]interface{}{
			"fileId": "pdf-1",
		})
		if err != nil {
			t.Fatalf("file info failed: %v", err)
		}

		data := extractResultJSON(t, result)
		if data["name"] != "Report.pdf" {
			t.Errorf("expected name=Report.pdf, got %v", data["name"])
		}
		if data["mimeType"] != "application/pdf" {
			t.Errorf("expected mimeType=application/pdf, got %v", data["mimeType"])
		}
	})

	t.Run("google doc", func(t *testing.T) {
		result, err := callTool(t, srv, "drive_file_info", map[string]interface{}{
			"fileId": "doc-1",
		})
		if err != nil {
			t.Fatalf("file info failed: %v", err)
		}

		data := extractResultJSON(t, result)
		if data["name"] != "Test Document" {
			t.Errorf("expected name=Test Document, got %v", data["name"])
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := callTool(t, srv, "drive_file_info", map[string]interface{}{
			"fileId": "nonexistent",
		})
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})
}

// --- drive_download_url ---

func TestDriveDownloadURL(t *testing.T) {
	srv := setupToolTest(t)

	t.Run("workspace file rejected", func(t *testing.T) {
		_, err := callTool(t, srv, "drive_download_url", map[string]interface{}{
			"fileId": "doc-1",
		})
		if err == nil {
			t.Error("expected error for workspace file")
		}
	})

	t.Run("regular file needs access token", func(t *testing.T) {
		// Without auth context, regular files fail at token extraction step
		_, err := callTool(t, srv, "drive_download_url", map[string]interface{}{
			"fileId": "pdf-1",
		})
		if err == nil {
			t.Error("expected error (no access token in test context)")
		}
	})
}

// --- drive_export_url ---

func TestDriveExportURL(t *testing.T) {
	srv := setupToolTest(t)

	t.Run("non-workspace file rejected", func(t *testing.T) {
		_, err := callTool(t, srv, "drive_export_url", map[string]interface{}{
			"fileId": "pdf-1",
			"format": "pdf",
		})
		if err == nil {
			t.Error("expected error for non-workspace file")
		}
	})

	t.Run("workspace file needs access token", func(t *testing.T) {
		// Without auth context, workspace files fail at token extraction step
		_, err := callTool(t, srv, "drive_export_url", map[string]interface{}{
			"fileId": "doc-1",
			"format": "pdf",
		})
		if err == nil {
			t.Error("expected error (no access token in test context)")
		}
	})
}

// --- drive_read_content (NEW) ---

func TestDriveReadContent(t *testing.T) {
	srv := setupToolTest(t)

	t.Run("google doc as text", func(t *testing.T) {
		result, err := callTool(t, srv, "drive_read_content", map[string]interface{}{
			"fileId": "doc-1",
		})
		if err != nil {
			t.Fatalf("read content failed: %v", err)
		}

		data := extractResultJSON(t, result)
		content, ok := data["content"].(string)
		if !ok || content == "" {
			t.Error("expected non-empty content")
		}
		if data["truncated"] != false {
			t.Error("expected truncated=false")
		}
	})

	t.Run("google sheet as csv", func(t *testing.T) {
		result, err := callTool(t, srv, "drive_read_content", map[string]interface{}{
			"fileId": "sheet-1",
		})
		if err != nil {
			t.Fatalf("read content failed: %v", err)
		}

		data := extractResultJSON(t, result)
		content := data["content"].(string)
		if content == "" {
			t.Error("expected CSV content")
		}
	})

	t.Run("text file", func(t *testing.T) {
		result, err := callTool(t, srv, "drive_read_content", map[string]interface{}{
			"fileId": "txt-1",
		})
		if err != nil {
			t.Fatalf("read content failed: %v", err)
		}

		data := extractResultJSON(t, result)
		content := data["content"].(string)
		if content != "These are some plain text notes." {
			t.Errorf("unexpected content: %q", content)
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := callTool(t, srv, "drive_read_content", map[string]interface{}{
			"fileId": "nonexistent",
		})
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})
}

// --- drive_list_recent (NEW) ---

func TestDriveListRecent(t *testing.T) {
	srv := setupToolTest(t)

	t.Run("default sort", func(t *testing.T) {
		result, err := callTool(t, srv, "drive_list_recent", map[string]interface{}{})
		if err != nil {
			t.Fatalf("list recent failed: %v", err)
		}

		data := extractResultJSON(t, result)
		files, ok := data["files"].([]interface{})
		if !ok {
			t.Fatal("expected files array")
		}
		if len(files) == 0 {
			t.Error("expected files, got empty")
		}
	})

	t.Run("with lastModified sort", func(t *testing.T) {
		result, err := callTool(t, srv, "drive_list_recent", map[string]interface{}{
			"orderBy": "lastModified",
		})
		if err != nil {
			t.Fatalf("list recent failed: %v", err)
		}

		data := extractResultJSON(t, result)
		if _, ok := data["files"]; !ok {
			t.Error("expected files key in result")
		}
	})

	t.Run("with lastModifiedByMe sort", func(t *testing.T) {
		result, err := callTool(t, srv, "drive_list_recent", map[string]interface{}{
			"orderBy": "lastModifiedByMe",
		})
		if err != nil {
			t.Fatalf("list recent failed: %v", err)
		}

		data := extractResultJSON(t, result)
		if _, ok := data["files"]; !ok {
			t.Error("expected files key in result")
		}
	})

	t.Run("custom page size", func(t *testing.T) {
		result, err := callTool(t, srv, "drive_list_recent", map[string]interface{}{
			"pageSize": float64(2),
		})
		if err != nil {
			t.Fatalf("list recent failed: %v", err)
		}

		data := extractResultJSON(t, result)
		if _, ok := data["files"]; !ok {
			t.Error("expected files key in result")
		}
	})

	t.Run("invalid orderBy", func(t *testing.T) {
		_, err := callTool(t, srv, "drive_list_recent", map[string]interface{}{
			"orderBy": "invalid_sort",
		})
		if err == nil {
			t.Error("expected error for invalid orderBy")
		}
	})
}

// --- drive_download_content (NEW) ---

func TestDriveDownloadContent(t *testing.T) {
	srv := setupToolTest(t)

	t.Run("regular file", func(t *testing.T) {
		result, err := callTool(t, srv, "drive_download_content", map[string]interface{}{
			"fileId": "txt-1",
		})
		if err != nil {
			t.Fatalf("download content failed: %v", err)
		}

		data := extractResultJSON(t, result)
		if data["data"] == nil || data["data"] == "" {
			t.Error("expected base64 data")
		}
		size, ok := data["size"].(float64)
		if !ok || size == 0 {
			t.Error("expected non-zero size")
		}
	})

	t.Run("workspace file with export mime", func(t *testing.T) {
		result, err := callTool(t, srv, "drive_download_content", map[string]interface{}{
			"fileId":         "doc-1",
			"exportMimeType": "text/plain",
		})
		if err != nil {
			t.Fatalf("download content failed: %v", err)
		}

		data := extractResultJSON(t, result)
		if data["mimeType"] != "text/plain" {
			t.Errorf("expected mimeType=text/plain, got %v", data["mimeType"])
		}
	})

	t.Run("workspace file default export", func(t *testing.T) {
		result, err := callTool(t, srv, "drive_download_content", map[string]interface{}{
			"fileId": "sheet-1",
		})
		if err != nil {
			t.Fatalf("download content failed: %v", err)
		}

		data := extractResultJSON(t, result)
		if data["mimeType"] != "text/plain" {
			t.Errorf("expected mimeType=text/plain (default), got %v", data["mimeType"])
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := callTool(t, srv, "drive_download_content", map[string]interface{}{
			"fileId": "nonexistent",
		})
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})
}

// --- drive_activity_changes ---

func TestDriveActivityChanges(t *testing.T) {
	srv := setupToolTest(t)

	result, err := callTool(t, srv, "drive_activity_changes", map[string]interface{}{})
	if err != nil {
		t.Fatalf("activity changes failed: %v", err)
	}

	data := extractResultArray(t, result)
	if len(data) == 0 {
		t.Error("expected activity changes, got empty")
	}
}

// --- drive_activity_deleted ---

func TestDriveActivityDeleted(t *testing.T) {
	srv := setupToolTest(t)

	result, err := callTool(t, srv, "drive_activity_deleted", map[string]interface{}{})
	if err != nil {
		t.Fatalf("activity deleted failed: %v", err)
	}

	// May be empty since no trashed files in mock, but should not error
	_ = result
}

// --- drive_activity_history ---

func TestDriveActivityHistory(t *testing.T) {
	srv := setupToolTest(t)

	result, err := callTool(t, srv, "drive_activity_history", map[string]interface{}{})
	if err != nil {
		t.Fatalf("activity history failed: %v", err)
	}

	data := extractResultArray(t, result)
	if len(data) == 0 {
		t.Error("expected activity history, got empty")
	}
}

// --- drive_file_revisions ---

func TestDriveFileRevisions(t *testing.T) {
	srv := setupToolTest(t)

	t.Run("list revisions", func(t *testing.T) {
		result, err := callTool(t, srv, "drive_file_revisions", map[string]interface{}{
			"fileId": "doc-1",
		})
		if err != nil {
			t.Fatalf("file revisions failed: %v", err)
		}

		data := extractResultArray(t, result)
		if len(data) == 0 {
			t.Error("expected revisions, got empty")
		}
	})
}

// --- drive_delete ---

func TestDriveDelete(t *testing.T) {
	srv := setupToolTest(t)

	t.Run("delete file", func(t *testing.T) {
		result, err := callTool(t, srv, "drive_delete", map[string]interface{}{
			"fileId": "txt-1",
		})
		if err != nil {
			t.Fatalf("delete failed: %v", err)
		}

		data := extractResultJSON(t, result)
		if data["message"] != "File moved to trash" {
			t.Errorf("expected message='File moved to trash', got %v", data["message"])
		}
	})

	t.Run("delete nonexistent", func(t *testing.T) {
		_, err := callTool(t, srv, "drive_delete", map[string]interface{}{
			"fileId": "nonexistent",
		})
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})
}

// --- drive_rename ---

func TestDriveRename(t *testing.T) {
	srv := setupToolTest(t)

	t.Run("rename file", func(t *testing.T) {
		result, err := callTool(t, srv, "drive_rename", map[string]interface{}{
			"fileId":  "pdf-1",
			"newName": "Renamed Report.pdf",
		})
		if err != nil {
			t.Fatalf("rename failed: %v", err)
		}

		data := extractResultJSON(t, result)
		if data["name"] != "Renamed Report.pdf" {
			t.Errorf("expected name=Renamed Report.pdf, got %v", data["name"])
		}
	})

	t.Run("empty name", func(t *testing.T) {
		_, err := callTool(t, srv, "drive_rename", map[string]interface{}{
			"fileId":  "pdf-1",
			"newName": "",
		})
		if err == nil {
			t.Error("expected error for empty name")
		}
	})
}

// --- drive_move ---

func TestDriveMove(t *testing.T) {
	srv := setupToolTest(t)

	t.Run("move file", func(t *testing.T) {
		result, err := callTool(t, srv, "drive_move", map[string]interface{}{
			"fileId":         "pdf-1",
			"targetFolderId": "empty-folder",
		})
		if err != nil {
			t.Fatalf("move failed: %v", err)
		}

		data := extractResultJSON(t, result)
		if data["id"] != "pdf-1" {
			t.Errorf("expected id=pdf-1, got %v", data["id"])
		}
	})
}

// --- drive_copy ---

func TestDriveCopy(t *testing.T) {
	srv := setupToolTest(t)

	t.Run("copy with new name", func(t *testing.T) {
		result, err := callTool(t, srv, "drive_copy", map[string]interface{}{
			"fileId":  "pdf-1",
			"newName": "Report Copy.pdf",
		})
		if err != nil {
			t.Fatalf("copy failed: %v", err)
		}

		data := extractResultJSON(t, result)
		if data["name"] != "Report Copy.pdf" {
			t.Errorf("expected name=Report Copy.pdf, got %v", data["name"])
		}
	})

	t.Run("copy without new name", func(t *testing.T) {
		result, err := callTool(t, srv, "drive_copy", map[string]interface{}{
			"fileId": "pdf-1",
		})
		if err != nil {
			t.Fatalf("copy failed: %v", err)
		}

		data := extractResultJSON(t, result)
		if data["name"] == nil {
			t.Error("expected a name in the copy")
		}
	})
}

// --- drive_folder_create ---

func TestDriveFolderCreate(t *testing.T) {
	srv := setupToolTest(t)

	t.Run("create folder", func(t *testing.T) {
		result, err := callTool(t, srv, "drive_folder_create", map[string]interface{}{
			"name":           "New Folder",
			"parentFolderId": "root",
		})
		if err != nil {
			t.Fatalf("folder create failed: %v", err)
		}

		data := extractResultJSON(t, result)
		if data["name"] != "New Folder" {
			t.Errorf("expected name=New Folder, got %v", data["name"])
		}
	})

	t.Run("empty name", func(t *testing.T) {
		_, err := callTool(t, srv, "drive_folder_create", map[string]interface{}{
			"name": "",
		})
		if err == nil {
			t.Error("expected error for empty name")
		}
	})
}

// --- drive_permissions_list ---

func TestDrivePermissionsList(t *testing.T) {
	srv := setupToolTest(t)

	result, err := callTool(t, srv, "drive_permissions_list", map[string]interface{}{
		"fileId": "doc-1",
	})
	if err != nil {
		t.Fatalf("permissions list failed: %v", err)
	}

	data := extractResultArray(t, result)
	if len(data) == 0 {
		t.Error("expected permissions, got empty")
	}
}

// --- drive_permissions_update ---

func TestDrivePermissionsUpdate(t *testing.T) {
	srv := setupToolTest(t)

	t.Run("add user permission", func(t *testing.T) {
		result, err := callTool(t, srv, "drive_permissions_update", map[string]interface{}{
			"fileId": "doc-1",
			"action": "add",
			"type":   "user",
			"role":   "writer",
			"email":  "newuser@test.com",
		})
		if err != nil {
			t.Fatalf("permissions update failed: %v", err)
		}

		data := extractResultArray(t, result)
		if len(data) == 0 {
			t.Error("expected permissions in result")
		}
	})

	t.Run("add anyone permission", func(t *testing.T) {
		result, err := callTool(t, srv, "drive_permissions_update", map[string]interface{}{
			"fileId": "pdf-1",
			"action": "add",
			"type":   "anyone",
			"role":   "reader",
		})
		if err != nil {
			t.Fatalf("permissions update failed: %v", err)
		}

		data := extractResultArray(t, result)
		if len(data) == 0 {
			t.Error("expected permissions in result")
		}
	})

	t.Run("remove permission", func(t *testing.T) {
		result, err := callTool(t, srv, "drive_permissions_update", map[string]interface{}{
			"fileId":       "doc-1",
			"action":       "remove",
			"permissionId": "perm-reader",
		})
		if err != nil {
			t.Fatalf("permissions update failed: %v", err)
		}

		data := extractResultArray(t, result)
		if len(data) == 0 {
			t.Error("expected permissions in result")
		}
	})

	t.Run("invalid action", func(t *testing.T) {
		_, err := callTool(t, srv, "drive_permissions_update", map[string]interface{}{
			"fileId": "doc-1",
			"action": "invalid",
		})
		if err == nil {
			t.Error("expected error for invalid action")
		}
	})
}

// --- drive_create_upload_url ---

func TestDriveCreateUploadURL(t *testing.T) {
	srv := setupToolTest(t)

	t.Run("needs access token", func(t *testing.T) {
		// Without auth context, upload URL generation fails at token extraction
		_, err := callTool(t, srv, "drive_create_upload_url", map[string]interface{}{
			"fileName": "new-file.pdf",
			"folderId": "folder-1",
		})
		if err == nil {
			t.Error("expected error (no access token in test context)")
		}
	})
}
