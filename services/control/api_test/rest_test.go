package apitest

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"control/internal/config"
)

// get fetches a path from the harness's server and returns the status, the
// content type and the body.
func (h *harness) get(path string) (int, string, string) {
	h.t.Helper()
	response, err := h.server.Client().Get(h.url + path)
	if err != nil {
		h.t.Fatalf("GET %s: %v", path, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		h.t.Fatalf("read %s: %v", path, err)
	}
	return response.StatusCode, response.Header.Get("Content-Type"), string(body)
}

func TestHealthAnswersOK(t *testing.T) {
	harness := newHarness(t)

	status, contentType, body := harness.get("/health")
	if status != http.StatusOK || body != "ok" || !strings.HasPrefix(contentType, "text/plain") {
		t.Errorf("GET /health = %d %q %q, want 200 text/plain \"ok\"", status, contentType, body)
	}
}

// The served spec is the whole contract, /graphql included, even though the
// Go code generated from it leaves /graphql to gqlgen.
func TestOpenAPISpecIsServedWhole(t *testing.T) {
	harness := newHarness(t)

	status, _, body := harness.get("/openapi.json")
	if status != http.StatusOK {
		t.Fatalf("GET /openapi.json = %d", status)
	}
	var spec struct {
		Info  struct{ Version string }
		Paths map[string]any
	}
	if err := json.Unmarshal([]byte(body), &spec); err != nil {
		t.Fatalf("spec is not JSON: %v", err)
	}
	if spec.Info.Version != "test-version" {
		t.Errorf("info.version = %q, want the server's own version", spec.Info.Version)
	}
	for _, path := range []string{"/health", "/graphql"} {
		if _, ok := spec.Paths[path]; !ok {
			t.Errorf("spec has no %s", path)
		}
	}

	status, _, body = harness.get("/openapi")
	if status != http.StatusOK || !strings.Contains(body, `"/openapi.json"`) {
		t.Errorf("GET /openapi = %d, want the docs page pointing at /openapi.json", status)
	}
}

// Swagger UI comes out of the binary, not off a CDN: the page's own assets are
// served under it.
func TestSwaggerUIIsServedLocally(t *testing.T) {
	harness := newHarness(t)

	status, _, page := harness.get("/openapi/")
	if status != http.StatusOK {
		t.Fatalf("GET /openapi/ = %d", status)
	}
	if strings.Contains(page, "cdn.") || !strings.Contains(page, "/openapi/swagger-ui-bundle.js") {
		t.Errorf("the docs page should load its bundle from /openapi/, not a CDN")
	}
	status, contentType, _ := harness.get("/openapi/swagger-ui-bundle.js")
	if status != http.StatusOK || !strings.Contains(contentType, "javascript") {
		t.Errorf("GET /openapi/swagger-ui-bundle.js = %d %q, want 200 JavaScript", status, contentType)
	}
}

// The server has no development mode: the tools are always there, and which of
// them the outside world reaches is the proxy's call.
func TestPlaygroundIsAlwaysServed(t *testing.T) {
	harness := newHarness(t)

	if status, _, _ := harness.get("/graphql/playground"); status != http.StatusOK {
		t.Errorf("GET /graphql/playground = %d, want 200", status)
	}
}

const introspection = `{ __schema { queryType { name } } }`

func TestIntrospectionIsOnByDefault(t *testing.T) {
	harness := newHarness(t)

	answer := harness.browser().post(introspection, nil)
	if name := answer.text("__schema.queryType.name"); name != "Query" {
		t.Errorf("introspection answered %v %v, want the Query type", answer.Data, answer.Errors)
	}
}

// The flag closes introspection on every transport, the subscription socket
// included — the one a proxy filtering request bodies would never see.
func TestIntrospectionCanBeDisabled(t *testing.T) {
	harness := newHarness(t, func(configuration *config.Config) {
		configuration.DisableGraphQLIntrospection = true
	})
	browser := harness.browser()

	answer := browser.post(introspection, nil)
	if len(answer.Errors) == 0 || answer.text("__schema.queryType.name") != "" {
		t.Errorf("introspection over HTTP answered %v, want it refused", answer.Data)
	}

	socket := browser.mustConnect()
	socket.subscribe("1", introspection, nil)
	frame := socket.await("1")
	if frame.Type == "next" && len(frame.data(t).Errors) == 0 {
		t.Errorf("introspection over the socket answered %s, want it refused", frame.Payload)
	}
}
