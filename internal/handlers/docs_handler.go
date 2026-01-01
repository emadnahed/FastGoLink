package handlers

import (
	"embed"
	"net/http"
	"os"
	"path/filepath"
)

//go:embed templates/*.html
var templatesFS embed.FS

// DocsHandler handles API documentation endpoints.
type DocsHandler struct {
	baseURL     string
	specPath    string
	specContent []byte
}

// NewDocsHandler creates a new DocsHandler.
// If specPath is empty, it will look for docs/openapi.yaml in the current directory.
func NewDocsHandler(baseURL string, specPath string) *DocsHandler {
	if specPath == "" {
		specPath = "docs/openapi.yaml"
	}
	return &DocsHandler{
		baseURL:  baseURL,
		specPath: specPath,
	}
}

// NewDocsHandlerWithSpec creates a new DocsHandler with an embedded spec.
func NewDocsHandlerWithSpec(baseURL string, specContent []byte) *DocsHandler {
	return &DocsHandler{
		baseURL:     baseURL,
		specContent: specContent,
	}
}

// ScalarUI serves the Scalar API documentation UI.
func (h *DocsHandler) ScalarUI(w http.ResponseWriter, r *http.Request) {
	html, err := templatesFS.ReadFile("templates/scalar.html")
	if err != nil {
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
		// Try to find the spec relative to the executable
		execPath, execErr := os.Executable()
		if execErr != nil {
			// If we can't find the executable, we can't find the spec relative to it.
			http.Error(w, "OpenAPI specification not found", http.StatusNotFound)
			return
		}
		execDir := filepath.Dir(execPath)
		altPath := filepath.Join(execDir, h.specPath)

		content, err = os.ReadFile(altPath)
		if err != nil {
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
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(html)
}
