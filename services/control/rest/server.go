// Package rest answers the HTTP endpoints that are not GraphQL. Their contract
// is packages/control-specs/openapi.yaml, generated into generated.go; this
// package, like graph/, holds only what implements it.
package rest

import "context"

//go:generate go tool oapi-codegen -config ../oapi_codegen.yml ../../../packages/control-specs/openapi.yaml

// Server implements the generated StrictServerInterface.
type Server struct{}

var _ StrictServerInterface = Server{}

// GetHealth answers as long as the process serves HTTP, and checks nothing
// else: a liveness probe that also looked at the database would restart a
// healthy process over a database problem.
func (Server) GetHealth(context.Context, GetHealthRequestObject) (GetHealthResponseObject, error) {
	return GetHealth200TextResponse("ok"), nil
}
