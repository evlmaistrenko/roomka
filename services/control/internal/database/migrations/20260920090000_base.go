package migrations

import (
	"context"

	"github.com/uptrace/bun"

	"control/internal/model"
)

// The base schema: accounts, authentication, access control.
//
// There is no catalog of roles or permissions here on purpose. The role set, and
// the permissions each role grants, are declared in the GraphQL schema (@grants
// on the Role enum) and read from it at startup; only a user's own rows live in
// the database.
func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		// Everything that belongs to a user dies with them. There is nothing in
		// this schema a deleted account could sensibly leave behind: a role they
		// no longer hold, a session for an account that is gone, a preference
		// nobody will read.
		belongsToUser := `("user_id") REFERENCES "users" ("id") ON DELETE CASCADE`
		if err := create(ctx, db, model.All(), map[any]string{
			(*model.UserRole)(nil):       belongsToUser,
			(*model.UserPermission)(nil): belongsToUser,
			(*model.Session)(nil):        belongsToUser,
			(*model.UserPreference)(nil): belongsToUser,
		}); err != nil {
			return err
		}

		_, err := db.NewCreateIndex().
			Model((*model.Session)(nil)).
			Index("sessions_by_user").
			Column("user_id").
			Exec(ctx)
		return err
	}, func(ctx context.Context, db *bun.DB) error {
		return drop(ctx, db, model.All())
	})
}
