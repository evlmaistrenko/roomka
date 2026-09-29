package graph

import (
	"context"
	"time"

	"github.com/99designs/gqlgen/graphql"

	"control/internal/identity"
	"control/internal/schema"
)

// directiveAuth implements @auth: the caller must be authenticated, must hold
// the permission the field names, and — when the field asks for it — must be on
// a session that has recently proved a password.
//
// The permission is checked before the step-up, so a caller who may not perform
// the operation at all is never asked for a password first.
//
// This is the whole of access control that the schema declares. The other half —
// which ranks may act on which — is not expressible in a directive and lives
// with the accounts it is about, in internal/accounts.
func (r *Resolver) directiveAuth(ctx context.Context, _ any, next graphql.Resolver, permission *schema.Permission, stepUp bool) (any, error) {
	principal, ok := identity.PrincipalFromContext(ctx)
	if !ok {
		return nil, schema.ErrUnauthenticated
	}
	if permission != nil {
		granted, err := r.Accounts.HasPermission(ctx, principal.UserID, *permission)
		if err != nil {
			return nil, err
		}
		if !granted {
			return nil, schema.ErrAccessDenied
		}
	}
	if stepUp && !principal.SteppedUp(time.Now().UTC()) {
		return nil, schema.ErrStepUpRequired
	}
	return next(ctx)
}
