package cc

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:frontend/dist
var frontendFS embed.FS

// spaHandler serves the embedded React SPA with fallback to index.html
// for client-side routing.
type spaHandler struct {
	fs http.Handler
}

func newSPAHandler() *spaHandler {
	sub, err := fs.Sub(frontendFS, "frontend/dist")
	if err != nil {
		panic("failed to create sub filesystem for frontend/dist: " + err.Error())
	}

	return &spaHandler{
		fs: http.FileServer(http.FS(sub)),
	}
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Try to serve the file directly
	path := r.URL.Path

	// Strip leading slash for fs lookup
	fsPath := strings.TrimPrefix(path, "/")
	if fsPath == "" {
		fsPath = "index.html"
	}

	// Check if file exists in embedded FS
	sub, _ := fs.Sub(frontendFS, "frontend/dist")

	if _, err := fs.Stat(sub, fsPath); err != nil {
		// File not found - serve index.html for SPA routing
		r.URL.Path = "/"
	}

	h.fs.ServeHTTP(w, r)
}
