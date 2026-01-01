package handlers

import (
	"net/http"
	"os"
	"path/filepath"
)

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
	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>FastGoLink API Documentation</title>
    <meta name="description" content="FastGoLink - High-performance URL shortening service API documentation">
    <link rel="icon" type="image/svg+xml" href="data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 100 100'><text y='.9em' font-size='90'>🔗</text></svg>">
    <style>
        body {
            margin: 0;
            padding: 0;
        }
    </style>
</head>
<body>
    <script id="api-reference" data-url="/docs/openapi.yaml"></script>
    <script>
        var configuration = {
            theme: 'purple',
            layout: 'modern',
            showSidebar: true,
            searchHotKey: 'k',
            metaData: {
                title: 'FastGoLink API',
                description: 'High-performance URL shortening service',
                ogDescription: 'Production-grade URL shortening API with Redis caching and PostgreSQL persistence',
                ogTitle: 'FastGoLink API Documentation',
                twitterCard: 'summary_large_image'
            },
            spec: {
                url: '/docs/openapi.yaml'
            },
            hideModels: false,
            hideDownloadButton: false,
            hideDarkModeToggle: false,
            darkMode: true,
            forceDarkModeState: 'dark',
            customCss: ` + "`" + `
                .darklight-reference-promo { display: none !important; }
                .scalar-app { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; }
            ` + "`" + `
        }

        document.getElementById('api-reference').dataset.configuration = JSON.stringify(configuration)
    </script>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(html))
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
		execPath, _ := os.Executable()
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
	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>FastGoLink API Documentation - ReDoc</title>
    <meta name="description" content="FastGoLink - High-performance URL shortening service API documentation">
    <link rel="icon" type="image/svg+xml" href="data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 100 100'><text y='.9em' font-size='90'>🔗</text></svg>">
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
    <style>
        body {
            margin: 0;
            padding: 0;
        }
    </style>
</head>
<body>
    <redoc spec-url='/docs/openapi.yaml'
           hide-hostname="false"
           expand-responses="200,201"
           path-in-middle-panel="true"
           hide-download-button="false"
           native-scrollbars="true"
           theme='{
               "colors": {
                   "primary": { "main": "#7c3aed" }
               },
               "typography": {
                   "fontFamily": "Inter, -apple-system, BlinkMacSystemFont, sans-serif",
                   "headings": { "fontFamily": "Inter, -apple-system, BlinkMacSystemFont, sans-serif" }
               },
               "sidebar": {
                   "backgroundColor": "#1e1e2e"
               }
           }'>
    </redoc>
    <script src="https://cdn.redoc.ly/redoc/latest/bundles/redoc.standalone.js"></script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(html))
}

// SwaggerUI serves the Swagger UI as another alternative.
func (h *DocsHandler) SwaggerUI(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>FastGoLink API Documentation - Swagger UI</title>
    <meta name="description" content="FastGoLink - High-performance URL shortening service API documentation">
    <link rel="icon" type="image/svg+xml" href="data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 100 100'><text y='.9em' font-size='90'>🔗</text></svg>">
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
    <style>
        html { box-sizing: border-box; overflow-y: scroll; }
        *, *:before, *:after { box-sizing: inherit; }
        body { margin: 0; padding: 0; background: #fafafa; }
        .swagger-ui .topbar { display: none; }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js" charset="UTF-8"></script>
    <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-standalone-preset.js" charset="UTF-8"></script>
    <script>
        window.onload = function() {
            window.ui = SwaggerUIBundle({
                url: "/docs/openapi.yaml",
                dom_id: '#swagger-ui',
                deepLinking: true,
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIStandalonePreset
                ],
                plugins: [
                    SwaggerUIBundle.plugins.DownloadUrl
                ],
                layout: "StandaloneLayout",
                defaultModelsExpandDepth: 1,
                defaultModelExpandDepth: 1,
                docExpansion: "list",
                filter: true,
                showExtensions: true,
                showCommonExtensions: true,
                tryItOutEnabled: true
            });
        };
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(html))
}
