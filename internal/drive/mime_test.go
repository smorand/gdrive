package drive

import "testing"

func TestDetectMimeType(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		// OOXML must override the ZIP signature heuristic.
		{"slides.pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation"},
		{"doc.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		{"sheet.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		// Case-insensitive extension lookup.
		{"REPORT.PDF", "application/pdf"},
		{"NOTE.MD", "text/markdown"},
		// Plain text family.
		{"readme.md", "text/markdown"},
		{"data.csv", "text/csv"},
		{"config.yaml", "application/yaml"},
		// Archives still resolve correctly.
		{"backup.zip", "application/zip"},
		// Unknown / no extension fall back to octet-stream.
		{"unknownfile", "application/octet-stream"},
		{"weird.xyz", "application/octet-stream"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DetectMimeType(tc.name)
			if got != tc.want {
				t.Fatalf("DetectMimeType(%q) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestDetectConversionTarget(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		// Word-processing → Docs
		{"spec.md", DriveDocMimeType},
		{"NOTES.TXT", DriveDocMimeType},
		{"page.html", DriveDocMimeType},
		{"page.htm", DriveDocMimeType},
		{"letter.rtf", DriveDocMimeType},
		{"old.doc", DriveDocMimeType},
		{"new.docx", DriveDocMimeType},
		{"libre.odt", DriveDocMimeType},

		// Spreadsheets → Sheets
		{"data.csv", DriveSheetMimeType},
		{"data.tsv", DriveSheetMimeType},
		{"book.xls", DriveSheetMimeType},
		{"book.xlsx", DriveSheetMimeType},
		{"calc.ods", DriveSheetMimeType},

		// Presentations → Slides
		{"deck.ppt", DriveSlideMimeType},
		{"deck.pptx", DriveSlideMimeType},
		{"impress.odp", DriveSlideMimeType},

		// Not convertible
		{"image.png", ""},
		{"video.mp4", ""},
		{"archive.zip", ""},
		{"noext", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DetectConversionTarget(tc.name)
			if got != tc.want {
				t.Fatalf("DetectConversionTarget(%q) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}
