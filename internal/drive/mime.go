package drive

import (
	"strings"
)

// extensionMimeTypes maps file extensions to their canonical MIME types.
//
// Why this exists: Office formats (.docx, .pptx, .xlsx, ...) are ZIP archives.
// Content sniffing (e.g. http.DetectContentType) returns "application/zip"
// for them, which causes Google Drive to display them with a generic ZIP icon
// and prevents Google Slides / Docs / Sheets from opening them. We force the
// correct OOXML / ODF MIME type from the extension.
var extensionMimeTypes = map[string]string{
	// Microsoft Office (OOXML, all start with PK signature)
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".dotx": "application/vnd.openxmlformats-officedocument.wordprocessingml.template",
	".docm": "application/vnd.ms-word.document.macroenabled.12",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".xltx": "application/vnd.openxmlformats-officedocument.spreadsheetml.template",
	".xlsm": "application/vnd.ms-excel.sheet.macroenabled.12",
	".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".potx": "application/vnd.openxmlformats-officedocument.presentationml.template",
	".ppsx": "application/vnd.openxmlformats-officedocument.presentationml.slideshow",
	".pptm": "application/vnd.ms-powerpoint.presentation.macroenabled.12",

	// Microsoft Office (legacy binary)
	".doc": "application/msword",
	".xls": "application/vnd.ms-excel",
	".ppt": "application/vnd.ms-powerpoint",

	// OpenDocument
	".odt": "application/vnd.oasis.opendocument.text",
	".ods": "application/vnd.oasis.opendocument.spreadsheet",
	".odp": "application/vnd.oasis.opendocument.presentation",

	// Documents
	".pdf":  "application/pdf",
	".epub": "application/epub+zip",
	".rtf":  "application/rtf",

	// Text and data
	".txt":  "text/plain",
	".md":   "text/markdown",
	".csv":  "text/csv",
	".tsv":  "text/tab-separated-values",
	".html": "text/html",
	".htm":  "text/html",
	".css":  "text/css",
	".json": "application/json",
	".xml":  "application/xml",
	".yaml": "application/yaml",
	".yml":  "application/yaml",

	// Images
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
	".bmp":  "image/bmp",
	".tiff": "image/tiff",
	".tif":  "image/tiff",
	".heic": "image/heic",
	".ico":  "image/x-icon",

	// Audio
	".mp3":  "audio/mpeg",
	".wav":  "audio/wav",
	".ogg":  "audio/ogg",
	".flac": "audio/flac",
	".m4a":  "audio/mp4",
	".aac":  "audio/aac",

	// Video
	".mp4":  "video/mp4",
	".mov":  "video/quicktime",
	".avi":  "video/x-msvideo",
	".mkv":  "video/x-matroska",
	".webm": "video/webm",
	".m4v":  "video/x-m4v",

	// Archives (kept LAST so Office extensions above win)
	".zip": "application/zip",
	".gz":  "application/gzip",
	".tar": "application/x-tar",
	".7z":  "application/x-7z-compressed",
	".rar": "application/vnd.rar",
}

// DetectMimeType returns the canonical MIME type for a filename based on its
// extension. Falls back to "application/octet-stream" when the extension is
// unknown, which lets Drive run its own server-side detection.
func DetectMimeType(filename string) string {
	idx := strings.LastIndex(filename, ".")
	if idx < 0 {
		return "application/octet-stream"
	}
	ext := strings.ToLower(filename[idx:])
	if mt, ok := extensionMimeTypes[ext]; ok {
		return mt
	}
	return "application/octet-stream"
}

// conversionTargets maps source extensions to the Google Workspace MIME type
// that Drive can convert them to during upload (when --convert is set).
//
// Drive performs server-side conversion when Files.Create is called with
// metadata.MimeType set to a google-apps.* type and the media body is
// uploaded with a compatible source ContentType.
var conversionTargets = map[string]string{
	// Word-processing → Google Docs
	".md":   DriveDocMimeType,
	".txt":  DriveDocMimeType,
	".html": DriveDocMimeType,
	".htm":  DriveDocMimeType,
	".rtf":  DriveDocMimeType,
	".doc":  DriveDocMimeType,
	".docx": DriveDocMimeType,
	".odt":  DriveDocMimeType,

	// Spreadsheets → Google Sheets
	".csv":  DriveSheetMimeType,
	".tsv":  DriveSheetMimeType,
	".xls":  DriveSheetMimeType,
	".xlsx": DriveSheetMimeType,
	".ods":  DriveSheetMimeType,

	// Presentations → Google Slides
	".ppt":  DriveSlideMimeType,
	".pptx": DriveSlideMimeType,
	".odp":  DriveSlideMimeType,
}

// DetectConversionTarget returns the Google Workspace MIME type that Drive
// can convert filename to (based on its extension) when uploading with
// conversion enabled. Returns "" when the extension cannot be converted.
func DetectConversionTarget(filename string) string {
	idx := strings.LastIndex(filename, ".")
	if idx < 0 {
		return ""
	}
	ext := strings.ToLower(filename[idx:])
	return conversionTargets[ext]
}
