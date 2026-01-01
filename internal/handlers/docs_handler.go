package handlers

import (
	"embed"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gourl/gourl/pkg/logger"
)

//go:embed templates/*.html
var templatesFS embed.FS

// DocsHandler handles API documentation endpoints.
type DocsHandler struct {
	baseURL     string
	specPath    string
	specContent []byte
	log         *logger.Logger
}

// NewDocsHandler creates a new DocsHandler.
// If specPath is empty, it will look for docs/openapi.yaml in the current directory.
func NewDocsHandler(baseURL string, specPath string, log *logger.Logger) *DocsHandler {
	if specPath == "" {
		specPath = "docs/openapi.yaml"
	}
	return &DocsHandler{
		baseURL:  baseURL,
		specPath: specPath,
		log:      log,
	}
}

// NewDocsHandlerWithSpec creates a new DocsHandler with an embedded spec.
func NewDocsHandlerWithSpec(baseURL string, specContent []byte, log *logger.Logger) *DocsHandler {
	return &DocsHandler{
		baseURL:     baseURL,
		specContent: specContent,
		log:         log,
	}
}

// ScalarUI serves the Scalar API documentation UI.
func (h *DocsHandler) ScalarUI(w http.ResponseWriter, r *http.Request) {
	html, err := templatesFS.ReadFile("templates/scalar.html")
	if err != nil {
		if h.log != nil {
			h.log.Error("failed to read scalar template", "error", err)
		}
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(html)
}

// OpenAPISpec serves the OpenAPI specification YAML file.
func (h *DocsHandler) OpenAPISpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/x-yaml")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=3600")

	// If we have embedded content, use it
	if len(h.specContent) > 0 {
		w.WriteHeader(http.StatusOK)
		w.Write(h.specContent)
		return
	}

	// Otherwise read from file
	content, err := os.ReadFile(h.specPath)
	if err != nil {
		if h.log != nil {
			h.log.Warn("failed to read OpenAPI spec, trying alternative path", "path", h.specPath, "error", err)
		}

		// Try to find the spec relative to the executable
		execPath, execErr := os.Executable()
		if execErr != nil {
			if h.log != nil {
				h.log.Error("failed to get executable path", "error", execErr)
			}
			http.Error(w, "OpenAPI specification not found", http.StatusNotFound)
			return
		}
		execDir := filepath.Dir(execPath)
		altPath := filepath.Join(execDir, h.specPath)

		content, err = os.ReadFile(altPath)
		if err != nil {
			if h.log != nil {
				h.log.Error("failed to read OpenAPI spec from alternative path", "path", altPath, "error", err)
			}
			http.Error(w, "OpenAPI specification not found", http.StatusNotFound)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

// Redoc serves the ReDoc API documentation UI as an alternative.
func (h *DocsHandler) Redoc(w http.ResponseWriter, r *http.Request) {
	html, err := templatesFS.ReadFile("templates/redoc.html")
	if err != nil {
		if h.log != nil {
			h.log.Error("failed to read redoc template", "error", err)
		}
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(html)
}

// SwaggerUI serves the Swagger UI as another alternative.
func (h *DocsHandler) SwaggerUI(w http.ResponseWriter, r *http.Request) {
	html, err := templatesFS.ReadFile("templates/swagger.html")
	if err != nil {
		if h.log != nil {
			h.log.Error("failed to read swagger template", "error", err)
		}
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(html)
}
