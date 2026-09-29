package graph

import (
	"github.com/vektah/gqlparser/v2/ast"

	"control/internal/accounts"
)

//go:generate go tool gqlgen generate

// Resolver is the root GraphQL resolver, and the whole of what this package
// knows how to do: hand each field to the service that answers it. Everything
// with a decision in it lives on the other side of that call.
type Resolver struct {
	Accounts *accounts.Service
}

// Directives implements the schema's directives: @auth in access.go, @constraint
// in validation.go.
//
// Grants is deliberately absent: gqlgen wires directive hooks only for a fixed
// set of locations, and ENUM_VALUE is not among them, so a Grants hook would
// never be called. The role→permission map declared by @grants is read from the
// parsed schema at startup instead — see internal/rbac.
func (r *Resolver) Directives() DirectiveRoot {
	return DirectiveRoot{
		Auth:       r.directiveAuth,
		Constraint: directiveConstraint,
	}
}

// Schema returns the parsed schema. It is available before a Resolver exists,
// which matters because the role→permission map is read out of the schema and
// therefore has to be built before the service that depends on it.
func Schema() *ast.Schema { return parsedSchema }
