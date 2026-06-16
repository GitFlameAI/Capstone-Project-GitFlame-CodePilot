package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSwaggerEndpoints(t *testing.T) {
	server := NewServer(Config{Addr: ":8000", MLServiceURL: "http://localhost:8001"})

	openAPIResponse := httptest.NewRecorder()
	openAPIRequest := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	server.Router().ServeHTTP(openAPIResponse, openAPIRequest)

	if openAPIResponse.Code != http.StatusOK {
		t.Fatalf("expected /openapi.json status 200, got %d", openAPIResponse.Code)
	}

	var spec map[string]any
	if err := json.Unmarshal(openAPIResponse.Body.Bytes(), &spec); err != nil {
		t.Fatalf("expected valid OpenAPI JSON: %v", err)
	}
	if spec["openapi"] != "3.0.3" {
		t.Fatalf("unexpected openapi version: %v", spec["openapi"])
	}

	swaggerResponse := httptest.NewRecorder()
	swaggerRequest := httptest.NewRequest(http.MethodGet, "/swagger/", nil)
	server.Router().ServeHTTP(swaggerResponse, swaggerRequest)

	if swaggerResponse.Code != http.StatusOK {
		t.Fatalf("expected /swagger/ status 200, got %d", swaggerResponse.Code)
	}
	if !strings.Contains(swaggerResponse.Body.String(), "SwaggerUIBundle") {
		t.Fatal("expected Swagger UI HTML to initialize SwaggerUIBundle")
	}
}
