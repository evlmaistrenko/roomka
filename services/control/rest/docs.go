package rest

import (
	"encoding/json"
	"net/http"

	"github.com/swaggest/swgui/v5emb"
)

// SpecHandler serves the whole OpenAPI spec as JSON, /graphql included: the
// spec is embedded before oapi-codegen leaves the graphql tag out of the Go
// code, so what is served is the contract as written. version replaces the
// spec's placeholder with the server's own.
func SpecHandler(version string) (http.Handler, error) {
	spec, err := GetSpec()
	if err != nil {
		return nil, err
	}
	spec.Info.Version = version
	body, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(body)
	}), nil
}

// DocsHandler serves Swagger UI at basePath over the spec at specPath. Its
// assets are embedded in the binary, so the page works with no network beyond
// this server. "Try it out" works against this server, cookie and all: the page
// is served from the API's own origin, which the Origin check lets through.
func DocsHandler(basePath, specPath string) http.Handler {
	return v5emb.New("roomka control", specPath, basePath)
}
