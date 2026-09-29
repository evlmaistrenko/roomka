// Package rbac resolves what a user is allowed to do.
//
// The role→permission map is not stored in the database. It is declared in the
// GraphQL schema by @grants on the Role enum and read from the parsed schema at
// startup, which keeps the schema the single source of truth for both server and
// client. gqlgen never invokes an ENUM_VALUE directive hook, so reading the AST
// is not a shortcut — it is the only way that declaration reaches the running
// server.
//
// A user's own rows (which roles they hold, which permissions were granted
// directly) do live in the database, and are read live on every check so a
// change takes effect on the next request rather than on the next login.
package rbac

import (
	"context"
	"fmt"
	"slices"

	"github.com/vektah/gqlparser/v2/ast"

	"control/internal/database"
	"control/internal/model"
)

// Roles is the role→permission map declared by the schema, plus the database
// handle used to read a user's own rows.
type Roles struct {
	store  *database.Database
	grants map[string][]string
}

// Load reads @grants off the Role enum and validates every literal it names.
//
// The validation is not decoration: nothing else checks these. A directive
// argument is not type-checked by the GraphQL parser, so a permission that does
// not exist parses as a perfectly valid schema, and — because the hook is never
// called — would otherwise be caught by nothing at all, at no point, ever.
func Load(schema *ast.Schema, store *database.Database) (*Roles, error) {
	roleEnum, err := enumValues(schema, "Role")
	if err != nil {
		return nil, err
	}
	permissionEnum, err := enumValues(schema, "Permission")
	if err != nil {
		return nil, err
	}
	known := make(map[string]bool, len(permissionEnum))
	for _, value := range permissionEnum {
		known[value.Name] = true
	}

	grants := make(map[string][]string, len(roleEnum))
	for _, role := range roleEnum {
		directive := role.Directives.ForName("grants")
		if directive == nil {
			grants[role.Name] = nil
			continue
		}
		argument := directive.Arguments.ForName("permissions")
		if argument == nil || argument.Value == nil {
			return nil, fmt.Errorf("rbac: @grants on Role.%s has no permissions argument", role.Name)
		}
		var permissions []string
		for _, child := range argument.Value.Children {
			name := child.Value.Raw
			if !known[name] {
				return nil, fmt.Errorf("rbac: @grants on Role.%s names %q, which is not a Permission", role.Name, name)
			}
			permissions = append(permissions, name)
		}
		grants[role.Name] = permissions
	}
	return &Roles{store: store, grants: grants}, nil
}

// enumValues returns the values of a named enum, failing if it is missing or is
// not an enum.
func enumValues(schema *ast.Schema, name string) (ast.EnumValueList, error) {
	definition, ok := schema.Types[name]
	if !ok {
		return nil, fmt.Errorf("rbac: schema has no type %s", name)
	}
	if definition.Kind != ast.Enum {
		return nil, fmt.Errorf("rbac: schema type %s is %s, want enum", name, definition.Kind)
	}
	return definition.EnumValues, nil
}

// Granted returns the permissions a role carries.
func (r *Roles) Granted(role string) []string { return r.grants[role] }

// UserRoles returns the roles held by userID.
func (r *Roles) UserRoles(ctx context.Context, userID int64) ([]string, error) {
	roles := []string{}
	err := r.store.NewSelect().
		Model((*model.UserRole)(nil)).
		Column("role").
		Where("user_id = ?", userID).
		Order("role").
		Scan(ctx, &roles)
	return roles, err
}

// DirectPermissions returns the permissions granted to userID outside any role.
func (r *Roles) DirectPermissions(ctx context.Context, userID int64) ([]string, error) {
	permissions := []string{}
	err := r.store.NewSelect().
		Model((*model.UserPermission)(nil)).
		Column("permission").
		Where("user_id = ?", userID).
		Order("permission").
		Scan(ctx, &permissions)
	return permissions, err
}

// EffectivePermissions returns the union of the permissions userID holds
// directly and through their roles.
func (r *Roles) EffectivePermissions(ctx context.Context, userID int64) ([]string, error) {
	direct, err := r.DirectPermissions(ctx, userID)
	if err != nil {
		return nil, err
	}
	roles, err := r.UserRoles(ctx, userID)
	if err != nil {
		return nil, err
	}
	effective := slices.Clone(direct)
	for _, role := range roles {
		for _, permission := range r.grants[role] {
			if !slices.Contains(effective, permission) {
				effective = append(effective, permission)
			}
		}
	}
	slices.Sort(effective)
	return effective, nil
}

// Has reports whether userID holds a permission, directly or through a role.
func (r *Roles) Has(ctx context.Context, userID int64, permission string) (bool, error) {
	effective, err := r.EffectivePermissions(ctx, userID)
	if err != nil {
		return false, err
	}
	return slices.Contains(effective, permission), nil
}
