package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"
)

// RegisterDocsRoutes registers Swagger UI and OpenAPI JSON endpoint routes.
// Routes are not included in the main Swagger documentation.
func (h *Handler) RegisterDocsRoutes(r chi.Router) {
	r.Get("/docs", h.HandleSwaggerUI)
	r.Get("/docs/openapi.json", h.HandleOpenAPIJSON)
}

// HandleSwaggerUI serves Swagger UI HTML pointing to /docs/openapi.json.
func (h *Handler) HandleSwaggerUI(w http.ResponseWriter, r *http.Request) {
	swaggerHTML := `
<!DOCTYPE html>
<html>
<head>
  <title>Pociag Gateway (BFF) API</title>
  <meta charset="utf-8"/>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@3/swagger-ui.css" />
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@3/swagger-ui.js"></script>
  <script>
    window.onload = function() {
      SwaggerUIBundle({
        url: "/docs/openapi.json",
        dom_id: '#swagger-ui',
        presets: [
          SwaggerUIBundle.presets.apis,
          SwaggerUIBundle.SwaggerUIStandalonePreset
        ],
        layout: "BaseLayout"
      })
    }
  </script>
</body>
</html>
`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(swaggerHTML))
}

// HandleOpenAPIJSON loads the OpenAPI spec from the filesystem, converts it from YAML to JSON, and serves it.
func (h *Handler) HandleOpenAPIJSON(w http.ResponseWriter, r *http.Request) {
	// Try multiple possible paths for the spec file
	possiblePaths := []string{
		"specs/openapi/gateway.yml",
		"../../../specs/openapi/gateway.yml",
		"../../../../specs/openapi/gateway.yml",
	}

	var specData []byte
	var err error

	// Find the spec file by trying multiple paths
	for _, path := range possiblePaths {
		specData, err = os.ReadFile(path)
		if err == nil {
			break
		}
	}

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("failed to read OpenAPI spec: %v", err),
		})
		return
	}

	var spec interface{}
	if err := yaml.Unmarshal(specData, &spec); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("failed to parse OpenAPI spec: %v", err),
		})
		return
	}

	jsonData, err := json.Marshal(spec)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("failed to convert spec to JSON: %v", err),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(jsonData)
}
