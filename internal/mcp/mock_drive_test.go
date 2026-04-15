package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockFile represents a file in the mock Drive API.
type mockFile struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	MimeType     string   `json:"mimeType"`
	Size         int64    `json:"size"`
	ModifiedTime string   `json:"modifiedTime"`
	CreatedTime  string   `json:"createdTime"`
	Parents      []string `json:"parents"`
	WebViewLink  string   `json:"webViewLink"`
	Trashed      bool     `json:"trashed"`
	TrashedTime  string   `json:"trashedTime,omitempty"`
	Content      []byte   `json:"-"` // raw file content
}

// mockPermission represents a permission.
type mockPermission struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	Role         string `json:"role"`
	EmailAddress string `json:"emailAddress,omitempty"`
	DisplayName  string `json:"displayName,omitempty"`
}

// mockRevision represents a file revision.
type mockRevision struct {
	ID                string `json:"id"`
	ModifiedTime      string `json:"modifiedTime"`
	Size              int64  `json:"size"`
	LastModifyingUser struct {
		DisplayName string `json:"displayName"`
	} `json:"lastModifyingUser"`
}

// mockDriveData holds all test data for the mock server.
type mockDriveData struct {
	files       map[string]*mockFile
	permissions map[string][]*mockPermission
	revisions   map[string][]*mockRevision
}

func newMockDriveData() *mockDriveData {
	return &mockDriveData{
		files: map[string]*mockFile{
			"doc-1": {
				ID: "doc-1", Name: "Test Document",
				MimeType: "application/vnd.google-apps.document", Size: 0,
				ModifiedTime: "2026-04-10T10:00:00Z", CreatedTime: "2026-04-01T08:00:00Z",
				Parents: []string{"folder-1"}, WebViewLink: "https://docs.google.com/document/d/doc-1",
				Content: []byte("This is the document content as plain text."),
			},
			"sheet-1": {
				ID: "sheet-1", Name: "Test Spreadsheet",
				MimeType: "application/vnd.google-apps.spreadsheet", Size: 0,
				ModifiedTime: "2026-04-11T12:00:00Z", CreatedTime: "2026-04-02T09:00:00Z",
				Parents: []string{"folder-1"}, WebViewLink: "https://docs.google.com/spreadsheets/d/sheet-1",
				Content: []byte("col1,col2\nval1,val2\n"),
			},
			"slides-1": {
				ID: "slides-1", Name: "Test Presentation",
				MimeType: "application/vnd.google-apps.presentation", Size: 0,
				ModifiedTime: "2026-04-12T14:00:00Z", CreatedTime: "2026-04-03T10:00:00Z",
				Parents: []string{"folder-1"}, WebViewLink: "https://docs.google.com/presentation/d/slides-1",
				Content: []byte("Slide 1: Title\nSlide 2: Content"),
			},
			"pdf-1": {
				ID: "pdf-1", Name: "Report.pdf",
				MimeType: "application/pdf", Size: 2048,
				ModifiedTime: "2026-04-09T15:00:00Z", CreatedTime: "2026-03-15T11:00:00Z",
				Parents: []string{"folder-1"}, WebViewLink: "https://drive.google.com/file/d/pdf-1",
				Content: []byte("%PDF-1.4 mock pdf content"),
			},
			"txt-1": {
				ID: "txt-1", Name: "notes.txt",
				MimeType: "text/plain", Size: 128,
				ModifiedTime: "2026-04-14T09:00:00Z", CreatedTime: "2026-04-14T08:00:00Z",
				Parents: []string{"root"}, WebViewLink: "https://drive.google.com/file/d/txt-1",
				Content: []byte("These are some plain text notes."),
			},
			"img-1": {
				ID: "img-1", Name: "photo.png",
				MimeType: "image/png", Size: 4096,
				ModifiedTime: "2026-04-13T16:00:00Z", CreatedTime: "2026-04-13T16:00:00Z",
				Parents: []string{"root"}, WebViewLink: "https://drive.google.com/file/d/img-1",
				Content: []byte{0x89, 0x50, 0x4E, 0x47}, // PNG header
			},
			"folder-1": {
				ID: "folder-1", Name: "Test Folder",
				MimeType: "application/vnd.google-apps.folder", Size: 0,
				ModifiedTime: "2026-04-10T10:00:00Z", CreatedTime: "2026-04-01T07:00:00Z",
				Parents: []string{"root"}, WebViewLink: "https://drive.google.com/drive/folders/folder-1",
			},
			"empty-folder": {
				ID: "empty-folder", Name: "Empty Folder",
				MimeType: "application/vnd.google-apps.folder", Size: 0,
				ModifiedTime: "2026-04-05T10:00:00Z", CreatedTime: "2026-04-05T10:00:00Z",
				Parents: []string{"root"}, WebViewLink: "https://drive.google.com/drive/folders/empty-folder",
			},
		},
		permissions: map[string][]*mockPermission{
			"doc-1": {
				{ID: "perm-owner", Type: "user", Role: "owner", EmailAddress: "owner@test.com", DisplayName: "Owner"},
				{ID: "perm-reader", Type: "user", Role: "reader", EmailAddress: "reader@test.com", DisplayName: "Reader"},
			},
			"pdf-1": {
				{ID: "perm-owner2", Type: "user", Role: "owner", EmailAddress: "owner@test.com", DisplayName: "Owner"},
			},
		},
		revisions: map[string][]*mockRevision{
			"doc-1": {
				{ID: "rev-1", ModifiedTime: "2026-04-01T08:00:00Z", Size: 100},
				{ID: "rev-2", ModifiedTime: "2026-04-10T10:00:00Z", Size: 200},
			},
		},
	}
}

// newMockDriveServer creates a mock Google Drive API server.
func newMockDriveServer(t *testing.T) (*httptest.Server, *mockDriveData) {
	t.Helper()
	data := newMockDriveData()

	mux := http.NewServeMux()

	// GET /files - List files (search/list)
	mux.HandleFunc("GET /files", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		orderBy := r.URL.Query().Get("orderBy")
		_ = orderBy

		var files []map[string]interface{}
		for _, f := range data.files {
			if f.Trashed {
				if !strings.Contains(q, "trashed = true") {
					continue
				}
			} else if strings.Contains(q, "trashed = true") {
				continue
			}

			// Simple name search filter
			if strings.Contains(q, "name contains") {
				nameStart := strings.Index(q, "'")
				nameEnd := strings.LastIndex(q, "'")
				if nameStart >= 0 && nameEnd > nameStart {
					searchTerm := q[nameStart+1 : nameEnd]
					if !strings.Contains(strings.ToLower(f.Name), strings.ToLower(searchTerm)) {
						continue
					}
				}
			}

			// Filter by parent
			if strings.Contains(q, "in parents") {
				parentStart := strings.Index(q, "'")
				parentEnd := strings.Index(q[parentStart+1:], "'") + parentStart + 1
				if parentStart >= 0 && parentEnd > parentStart {
					parentID := q[parentStart+1 : parentEnd]
					found := false
					for _, p := range f.Parents {
						if p == parentID {
							found = true
							break
						}
					}
					if !found {
						continue
					}
				}
			}

			files = append(files, map[string]interface{}{
				"id": f.ID, "name": f.Name, "mimeType": f.MimeType,
				"modifiedTime": f.ModifiedTime, "size": fmt.Sprintf("%d", f.Size),
				"owners":      []map[string]string{{"displayName": "Test Owner"}},
				"webViewLink": f.WebViewLink,
			})
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"files":         files,
			"nextPageToken": "",
		})
	})

	// GET /files/{id} - Get file metadata or download
	mux.HandleFunc("GET /files/", func(w http.ResponseWriter, r *http.Request) {
		// Extract file ID from path
		path := strings.TrimPrefix(r.URL.Path, "/files/")
		parts := strings.SplitN(path, "/", 2)
		fileID := parts[0]

		// Check for sub-resources
		if len(parts) > 1 {
			switch {
			case strings.HasPrefix(parts[1], "permissions"):
				handlePermissions(w, r, fileID, parts[1], data)
				return
			case strings.HasPrefix(parts[1], "revisions"):
				handleRevisions(w, r, fileID, data)
				return
			case strings.HasPrefix(parts[1], "export"):
				handleExport(w, r, fileID, data)
				return
			}
		}

		f, ok := data.files[fileID]
		if !ok {
			http.Error(w, `{"error":{"code":404,"message":"File not found"}}`, http.StatusNotFound)
			return
		}

		// Download (alt=media)
		if r.URL.Query().Get("alt") == "media" {
			w.Header().Set("Content-Type", f.MimeType)
			w.Write(f.Content)
			return
		}

		// Metadata
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": f.ID, "name": f.Name, "mimeType": f.MimeType,
			"size": fmt.Sprintf("%d", f.Size), "createdTime": f.CreatedTime,
			"modifiedTime": f.ModifiedTime, "webViewLink": f.WebViewLink,
			"parents": f.Parents,
			"owners":  []map[string]string{{"displayName": "Test Owner", "emailAddress": "owner@test.com"}},
		})
	})

	// PATCH /files/{id} - Update file (rename/move)
	mux.HandleFunc("PATCH /files/", func(w http.ResponseWriter, r *http.Request) {
		fileID := strings.TrimPrefix(r.URL.Path, "/files/")
		f, ok := data.files[fileID]
		if !ok {
			http.Error(w, `{"error":{"code":404,"message":"File not found"}}`, http.StatusNotFound)
			return
		}

		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)

		if name, ok := body["name"].(string); ok && name != "" {
			f.Name = name
		}

		// Handle move (addParents/removeParents)
		if addParents := r.URL.Query().Get("addParents"); addParents != "" {
			f.Parents = []string{addParents}
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": f.ID, "name": f.Name, "parents": f.Parents,
			"webViewLink": f.WebViewLink,
		})
	})

	// DELETE /files/{id}
	mux.HandleFunc("DELETE /files/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/files/")
		parts := strings.SplitN(path, "/", 2)
		fileID := parts[0]

		// Handle permission deletion: /files/{id}/permissions/{permId}
		if len(parts) > 1 && strings.HasPrefix(parts[1], "permissions/") {
			permID := strings.TrimPrefix(parts[1], "permissions/")
			perms, ok := data.permissions[fileID]
			if !ok {
				http.Error(w, `{"error":{"code":404,"message":"Not found"}}`, http.StatusNotFound)
				return
			}
			for i, p := range perms {
				if p.ID == permID {
					data.permissions[fileID] = append(perms[:i], perms[i+1:]...)
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
			http.Error(w, `{"error":{"code":404,"message":"Permission not found"}}`, http.StatusNotFound)
			return
		}

		if _, ok := data.files[fileID]; !ok {
			http.Error(w, `{"error":{"code":404,"message":"File not found"}}`, http.StatusNotFound)
			return
		}
		delete(data.files, fileID)
		w.WriteHeader(http.StatusNoContent)
	})

	// POST /files - Create file/folder
	mux.HandleFunc("POST /files/", func(w http.ResponseWriter, r *http.Request) {
		// Handle copy: POST /files/{id}/copy
		path := strings.TrimPrefix(r.URL.Path, "/files/")
		if strings.Contains(path, "/copy") {
			fileID := strings.TrimSuffix(path, "/copy")
			f, ok := data.files[fileID]
			if !ok {
				http.Error(w, `{"error":{"code":404,"message":"File not found"}}`, http.StatusNotFound)
				return
			}

			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)

			copyName := f.Name + " (copy)"
			if name, ok := body["name"].(string); ok && name != "" {
				copyName = name
			}

			copyID := fileID + "-copy"
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": copyID, "name": copyName, "webViewLink": f.WebViewLink,
			})
			return
		}

		// Handle permission creation: POST /files/{id}/permissions
		if strings.Contains(path, "/permissions") {
			fileID := strings.TrimSuffix(path, "/permissions")
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)

			perm := &mockPermission{
				ID:   fmt.Sprintf("perm-%d", len(data.permissions[fileID])+1),
				Type: body["type"].(string),
				Role: body["role"].(string),
			}
			if email, ok := body["emailAddress"].(string); ok {
				perm.EmailAddress = email
			}
			data.permissions[fileID] = append(data.permissions[fileID], perm)

			json.NewEncoder(w).Encode(map[string]interface{}{"id": perm.ID})
			return
		}
	})

	mux.HandleFunc("POST /files", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)

		name, _ := body["name"].(string)
		mimeType, _ := body["mimeType"].(string)
		parents, _ := body["parents"].([]interface{})

		newID := "new-" + name
		parentID := "root"
		if len(parents) > 0 {
			parentID = parents[0].(string)
		}

		data.files[newID] = &mockFile{
			ID: newID, Name: name, MimeType: mimeType,
			Parents: []string{parentID}, ModifiedTime: "2026-04-15T10:00:00Z",
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": newID, "name": name,
		})
	})

	// POST /upload/files - Upload
	mux.HandleFunc("POST /upload/files", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", r.URL.String()+"?uploadId=test-upload-id")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"id": "uploaded-file"})
	})

	// Activity API endpoints
	mux.HandleFunc("GET /changes/startPageToken", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"startPageToken": "100"})
	})

	mux.HandleFunc("GET /changes", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"changes": []map[string]interface{}{
				{
					"type":   "file",
					"fileId": "doc-1",
					"file": map[string]interface{}{
						"id": "doc-1", "name": "Test Document",
						"mimeType": "application/vnd.google-apps.document",
					},
					"time":    "2026-04-10T10:00:00Z",
					"removed": false,
				},
			},
			"newStartPageToken": "101",
		})
	})

	// Drive Activity API v2
	mux.HandleFunc("POST /v2/activity:query", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"activities": []map[string]interface{}{
				{
					"primaryActionDetail": map[string]interface{}{
						"edit": map[string]interface{}{},
					},
					"targets": []map[string]interface{}{
						{"driveItem": map[string]interface{}{"name": "items/doc-1", "title": "Test Document"}},
					},
					"actors": []map[string]interface{}{
						{"user": map[string]interface{}{"knownUser": map[string]interface{}{"personName": "people/user1"}}},
					},
					"timestamp": "2026-04-10T10:00:00Z",
				},
			},
		})
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts, data
}

func handlePermissions(w http.ResponseWriter, r *http.Request, fileID, subPath string, data *mockDriveData) {
	perms := data.permissions[fileID]
	var permList []map[string]interface{}
	for _, p := range perms {
		pm := map[string]interface{}{
			"id": p.ID, "type": p.Type, "role": p.Role,
		}
		if p.EmailAddress != "" {
			pm["emailAddress"] = p.EmailAddress
		}
		if p.DisplayName != "" {
			pm["displayName"] = p.DisplayName
		}
		permList = append(permList, pm)
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"permissions": permList})
}

func handleRevisions(w http.ResponseWriter, _ *http.Request, fileID string, data *mockDriveData) {
	revs := data.revisions[fileID]
	if revs == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"revisions": []interface{}{}})
		return
	}
	var revList []map[string]interface{}
	for _, r := range revs {
		revList = append(revList, map[string]interface{}{
			"id": r.ID, "modifiedTime": r.ModifiedTime, "size": fmt.Sprintf("%d", r.Size),
			"lastModifyingUser": map[string]string{"displayName": "Test User"},
		})
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"revisions": revList})
}

func handleExport(w http.ResponseWriter, r *http.Request, fileID string, data *mockDriveData) {
	f, ok := data.files[fileID]
	if !ok {
		http.Error(w, `{"error":{"code":404,"message":"File not found"}}`, http.StatusNotFound)
		return
	}
	exportMime := r.URL.Query().Get("mimeType")
	_ = exportMime
	w.Header().Set("Content-Type", "text/plain")
	w.Write(f.Content)
}
