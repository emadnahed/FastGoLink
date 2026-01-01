package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocsHandler_ScalarUI(t *testing.T) {
	handler := NewDocsHandler("http://localhost:8080", "", nil)

	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	rr := httptest.NewRecorder()

	handler.ScalarUI(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "text/html; charset=utf-8", rr.Header().Get("Content-Type"))

	body := rr.Body.String()
	assert.Contains(t, body, "<!DOCTYPE html>")
	assert.Contains(t, body, "FastGoLink API Documentation")
	assert.Contains(t, body, "/docs/openapi.yaml")
	assert.Contains(t, body, "@scalar/api-reference")
}

func TestDocsHandler_Redoc(t *testing.T) {
	handler := NewDocsHandler("http://localhost:8080", "", nil)

	req := httptest.NewRequest(http.MethodGet, "/docs/redoc", nil)
	rr := httptest.NewRecorder()

	handler.Redoc(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "text/html; charset=utf-8", rr.Header().Get("Content-Type"))

	body := rr.Body.String()
	assert.Contains(t, body, "<!DOCTYPE html>")
	assert.Contains(t, body, "FastGoLink API Documentation - ReDoc")
	assert.Contains(t, body, "/docs/openapi.yaml")
	assert.Contains(t, body, "redoc.standalone.js")
}

func TestDocsHandler_SwaggerUI(t *testing.T) {
	handler := NewDocsHandler("http://localhost:8080", "", nil)

	req := httptest.NewRequest(http.MethodGet, "/docs/swagger", nil)
	rr := httptest.NewRecorder()

	handler.SwaggerUI(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "text/html; charset=utf-8", rr.Header().Get("Content-Type"))

	body := rr.Body.String()
	assert.Contains(t, body, "<!DOCTYPE html>")
	assert.Contains(t, body, "FastGoLink API Documentation - Swagger UI")
	assert.Contains(t, body, "/docs/openapi.yaml")
	assert.Contains(t, body, "swagger-ui-bundle.js")
}

func TestDocsHandler_OpenAPISpec_WithEmbeddedContent(t *testing.T) {
	specContent := []byte(`openapi: "3.0.0"
info:
  title: Test API
  version: "1.0.0"`)

	handler := NewDocsHandlerWithSpec("http://localhost:8080", specContent, nil)

	req := httptest.NewRequest(http.MethodGet, "/docs/openapi.yaml", nil)
	rr := httptest.NewRecorder()

	handler.OpenAPISpec(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/x-yaml", rr.Header().Get("Content-Type"))
	assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, rr.Header().Get("Cache-Control"), "public")

	body := rr.Body.String()
	assert.Contains(t, body, "openapi:")
	assert.Contains(t, body, "Test API")
}

func TestDocsHandler_OpenAPISpec_FromFile(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "openapi.yaml")

	specContent := `openapi: "3.0.0"
info:
  title: FastGoLink API
  version: "1.0.0"`

	err := os.WriteFile(specPath, []byte(specContent), 0644)
	require.NoError(t, err)

	handler := NewDocsHandler("http://localhost:8080", specPath, nil)

	req := httptest.NewRequest(http.MethodGet, "/docs/openapi.yaml", nil)
	rr := httptest.NewRecorder()

	handler.OpenAPISpec(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/x-yaml", rr.Header().Get("Content-Type"))

	body := rr.Body.String()
	assert.Contains(t, body, "FastGoLink API")
}

func TestDocsHandler_OpenAPISpec_FileNotFound(t *testing.T) {
	handler := NewDocsHandler("http://localhost:8080", "/nonexistent/path/openapi.yaml", nil)

	req := httptest.NewRequest(http.MethodGet, "/docs/openapi.yaml", nil)
	rr := httptest.NewRecorder()

	handler.OpenAPISpec(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	assert.Contains(t, rr.Body.String(), "OpenAPI specification not found")
}

func TestNewDocsHandler_DefaultPath(t *testing.T) {
	handler := NewDocsHandler("http://localhost:8080", "", nil)
	assert.Equal(t, "docs/openapi.yaml", handler.specPath)
}

func TestNewDocsHandler_CustomPath(t *testing.T) {
	handler := NewDocsHandler("http://localhost:8080", "/custom/path/api.yaml", nil)
	assert.Equal(t, "/custom/path/api.yaml", handler.specPath)
}

func TestDocsHandler_ScalarUI_ContainsConfiguration(t *testing.T) {
	handler := NewDocsHandler("http://localhost:8080", "", nil)

	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	rr := httptest.NewRecorder()

	handler.ScalarUI(rr, req)

	body := rr.Body.String()

	// Verify Scalar configuration is present
	assert.Contains(t, body, "theme: 'purple'")
	assert.Contains(t, body, "layout: 'modern'")
	assert.Contains(t, body, "showSidebar: true")
	assert.Contains(t, body, "darkMode: true")
}

func TestDocsHandler_AllEndpoints_SetCorrectHeaders(t *testing.T) {
	handler := NewDocsHandlerWithSpec("http://localhost:8080", []byte("test"), nil)

	tests := []struct {
		name        string
		handlerFunc func(http.ResponseWriter, *http.Request)
		contentType string
	}{
		{
			name:        "ScalarUI",
			handlerFunc: handler.ScalarUI,
			contentType: "text/html; charset=utf-8",
		},
		{
			name:        "Redoc",
			handlerFunc: handler.Redoc,
			contentType: "text/html; charset=utf-8",
		},
		{
			name:        "SwaggerUI",
			handlerFunc: handler.SwaggerUI,
			contentType: "text/html; charset=utf-8",
		},
		{
			name:        "OpenAPISpec",
			handlerFunc: handler.OpenAPISpec,
			contentType: "application/x-yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rr := httptest.NewRecorder()

			tt.handlerFunc(rr, req)

			assert.Equal(t, http.StatusOK, rr.Code)
			assert.True(t, strings.HasPrefix(rr.Header().Get("Content-Type"), tt.contentType))
		})
	}
}
