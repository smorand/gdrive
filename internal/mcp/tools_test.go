package mcp

import (
	"testing"
)

func TestDetectMimeType(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"document.pdf", "application/pdf"},
		{"image.png", "image/png"},
		{"photo.jpg", "image/jpeg"},
		{"photo.jpeg", "image/jpeg"},
		{"data.csv", "text/csv"},
		{"page.html", "text/html"},
		{"config.json", "application/json"},
		{"archive.zip", "application/zip"},
		{"report.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		{"sheet.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		{"slides.pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation"},
		{"notes.txt", "text/plain"},
		{"video.mp4", "video/mp4"},
		{"song.mp3", "audio/mpeg"},
		{"diagram.svg", "image/svg+xml"},
		{"image.gif", "image/gif"},
		{"sound.wav", "audio/wav"},
		{"data.xml", "application/xml"},
		{"UPPER.PDF", "application/pdf"},
		{"unknown.xyz", "application/octet-stream"},
		{"noextension", "application/octet-stream"},
		{"multiple.dots.txt", "text/plain"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := detectMimeType(tt.filename)
			if got != tt.want {
				t.Errorf("detectMimeType(%q) = %q, want %q", tt.filename, got, tt.want)
			}
		})
	}
}

func TestToolResult(t *testing.T) {
	t.Run("marshals map", func(t *testing.T) {
		data := map[string]interface{}{"key": "value", "count": 42}
		result, err := toolResult(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil result")
		}
	})

	t.Run("marshals slice", func(t *testing.T) {
		data := []map[string]interface{}{
			{"id": "1", "name": "file1"},
			{"id": "2", "name": "file2"},
		}
		result, err := toolResult(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil result")
		}
	})

	t.Run("marshals empty slice", func(t *testing.T) {
		data := make([]map[string]interface{}, 0)
		result, err := toolResult(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil result")
		}
	})
}
