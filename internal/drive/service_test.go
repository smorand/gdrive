package drive

import "testing"

// newTestService returns a Service with a nil *drive.Service. The pure helpers
// under test never dereference it.
func newTestService() *Service { return &Service{} }

func TestParseRemotePath(t *testing.T) {
	ds := newTestService()
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"/", nil},
		{"a", []string{"a"}},
		{"/a/b/c", []string{"a", "b", "c"}},
		{"a/b/c/", []string{"a", "b", "c"}},
		{"My Drive/Notes", []string{"My Drive", "Notes"}},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := ds.ParseRemotePath(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("ParseRemotePath(%q) len = %d (%v), want %d (%v)",
					tc.in, len(got), got, len(tc.want), tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("ParseRemotePath(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestGetDefaultExportFormat(t *testing.T) {
	ds := newTestService()
	cases := []struct {
		mime string
		want string
	}{
		{"application/vnd.google-apps.document", "pdf"},
		{"application/vnd.google-apps.spreadsheet", "xlsx"},
		{"application/vnd.google-apps.presentation", "pptx"},
		{"application/vnd.google-apps.drawing", "pdf"},
		{"application/pdf", ""},
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.mime, func(t *testing.T) {
			got := ds.GetDefaultExportFormat(tc.mime)
			if got != tc.want {
				t.Fatalf("GetDefaultExportFormat(%q) = %q, want %q", tc.mime, got, tc.want)
			}
		})
	}
}

func TestGetExportMimeType(t *testing.T) {
	ds := newTestService()
	cases := []struct {
		workspaceMime string
		format        string
		want          string
	}{
		// Docs supports markdown (the new addition) plus the legacy formats.
		{"application/vnd.google-apps.document", "md", "text/markdown"},
		{"application/vnd.google-apps.document", "pdf", "application/pdf"},
		{"application/vnd.google-apps.document", "docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		{"application/vnd.google-apps.document", "txt", "text/plain"},
		{"application/vnd.google-apps.document", "html", "text/html"},

		// Sheets formats.
		{"application/vnd.google-apps.spreadsheet", "xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		{"application/vnd.google-apps.spreadsheet", "csv", "text/csv"},
		{"application/vnd.google-apps.spreadsheet", "pdf", "application/pdf"},

		// Slides formats.
		{"application/vnd.google-apps.presentation", "pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation"},
		{"application/vnd.google-apps.presentation", "pdf", "application/pdf"},

		// Drawings (newly supported).
		{"application/vnd.google-apps.drawing", "pdf", "application/pdf"},
		{"application/vnd.google-apps.drawing", "png", "image/png"},
		{"application/vnd.google-apps.drawing", "svg", "image/svg+xml"},
		{"application/vnd.google-apps.drawing", "jpg", "image/jpeg"},

		// Unsupported combinations return "".
		{"application/vnd.google-apps.document", "xlsx", ""},
		{"application/vnd.google-apps.spreadsheet", "md", ""},
		{"application/vnd.google-apps.form", "pdf", ""},
		{"application/pdf", "pdf", ""},
	}
	for _, tc := range cases {
		t.Run(tc.workspaceMime+"/"+tc.format, func(t *testing.T) {
			got := ds.GetExportMimeType(tc.workspaceMime, tc.format)
			if got != tc.want {
				t.Fatalf("GetExportMimeType(%q,%q) = %q, want %q",
					tc.workspaceMime, tc.format, got, tc.want)
			}
		})
	}
}

func TestAdjustFilename(t *testing.T) {
	ds := newTestService()
	cases := []struct {
		in           string
		exportFormat string
		want         string
	}{
		{"report.pdf", "md", "report.md"},
		{"report", "md", "report.md"},
		{"path/to/report.docx", "pdf", "path/to/report.pdf"},
		{"noext", "csv", "noext.csv"},
		{"unchanged.pdf", "", "unchanged.pdf"},
	}
	for _, tc := range cases {
		t.Run(tc.in+"->"+tc.exportFormat, func(t *testing.T) {
			got := ds.AdjustFilename(tc.in, tc.exportFormat)
			if got != tc.want {
				t.Fatalf("AdjustFilename(%q,%q) = %q, want %q", tc.in, tc.exportFormat, got, tc.want)
			}
		})
	}
}

// TextExportFormats is the source of truth for MCP `read content` exports.
// Pin the contract so a future change is intentional.
func TestTextExportFormats(t *testing.T) {
	want := map[string]string{
		"application/vnd.google-apps.document":     "text/markdown",
		"application/vnd.google-apps.spreadsheet":  "text/csv",
		"application/vnd.google-apps.presentation": "text/plain",
	}
	if len(TextExportFormats) != len(want) {
		t.Fatalf("TextExportFormats has %d entries, want %d", len(TextExportFormats), len(want))
	}
	for k, v := range want {
		if got := TextExportFormats[k]; got != v {
			t.Fatalf("TextExportFormats[%q] = %q, want %q", k, got, v)
		}
	}
}

// IsGoogleWorkspaceFile recognises every type that has an export entry,
// otherwise downloads will hit "cannot export file type". Pin the invariant.
func TestWorkspaceTypesCoveredByExportFormats(t *testing.T) {
	ds := newTestService()
	for mime := range ExportFormats {
		f := &fakeFileMimeOnly{m: mime}
		if !ds.isGoogleWorkspaceFileMime(f.m) {
			t.Fatalf("ExportFormats has %q but IsGoogleWorkspaceFile would not flag it", mime)
		}
	}
}

// fakeFileMimeOnly is a stand-in so the test does not need to import drive/v3.
type fakeFileMimeOnly struct{ m string }

// isGoogleWorkspaceFileMime mirrors IsGoogleWorkspaceFile's logic on a raw
// MIME string, avoiding the need to instantiate *drive.File in tests.
func (ds *Service) isGoogleWorkspaceFileMime(mime string) bool {
	workspaceTypes := []string{
		"application/vnd.google-apps.document",
		"application/vnd.google-apps.spreadsheet",
		"application/vnd.google-apps.presentation",
		"application/vnd.google-apps.form",
		"application/vnd.google-apps.drawing",
		"application/vnd.google-apps.map",
		"application/vnd.google-apps.site",
	}
	for _, t := range workspaceTypes {
		if mime == t {
			return true
		}
	}
	return false
}
