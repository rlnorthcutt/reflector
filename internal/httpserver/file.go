package httpserver

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// fileContentTypes maps common demo file extensions (lowercased, no dot)
// to a plausible Content-Type. It's deliberately small and hand-maintained
// rather than mime.TypeByExtension: that function falls back to its own
// small built-in table when the host has no /etc/mime.types, which a
// scratch container never does, so relying on it would make responses
// depend on the machine reflector happens to run on.
var fileContentTypes = map[string]string{
	"jpg":  "image/jpeg",
	"jpeg": "image/jpeg",
	"png":  "image/png",
	"gif":  "image/gif",
	"webp": "image/webp",
	"svg":  "image/svg+xml",
	"ico":  "image/x-icon",
	"pdf":  "application/pdf",
	"txt":  "text/plain",
	"csv":  "text/csv",
	"json": "application/json",
	"xml":  "application/xml",
	"html": "text/html",
	"htm":  "text/html",
	"css":  "text/css",
	"js":   "application/javascript",
	"zip":  "application/zip",
	"gz":   "application/gzip",
	"tar":  "application/x-tar",
	"doc":  "application/msword",
	"docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	"xls":  "application/vnd.ms-excel",
	"xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	"ppt":  "application/vnd.ms-powerpoint",
	"pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	"mp3":  "audio/mpeg",
	"mp4":  "video/mp4",
	"wasm": "application/wasm",
	"bin":  "application/octet-stream",
}

// fileExtPattern restricts the extension in /file/{bytes}.{ext} to a
// small, predictable charset — letters and digits only — so it can never
// carry a path separator or a header-injection character into the
// Content-Disposition filename.
var fileExtPattern = regexp.MustCompile(`^[A-Za-z0-9]{1,15}$`)

// handleFile streams exactly n zero-filled bytes as a named, typed "file"
// download: /file/{n}.{ext} sets Content-Type from ext (falling back to
// application/octet-stream for anything not in fileContentTypes) and
// Content-Disposition so a browser saves it as "{n}.{ext}". The content
// itself is not a valid file of that format — just n bytes with the right
// envelope for size, timeout, and download-behavior demos.
func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	spec := r.PathValue("spec")
	dot := strings.LastIndexByte(spec, '.')
	if dot < 0 {
		http.Error(w, "invalid file spec, expected {bytes}.{ext} e.g. /file/1024.jpg", http.StatusBadRequest)
		return
	}
	sizePart, ext := spec[:dot], spec[dot+1:]

	n, ok := parseByteCount(w, sizePart)
	if !ok {
		return
	}
	if !fileExtPattern.MatchString(ext) {
		http.Error(w, "invalid file extension, expected 1-15 letters/digits", http.StatusBadRequest)
		return
	}

	contentType, ok := fileContentTypes[strings.ToLower(ext)]
	if !ok {
		contentType = "application/octet-stream"
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.%s"`, sizePart, ext))
	writeExactBytes(w, n, contentType, zeroChunkNext)
}
