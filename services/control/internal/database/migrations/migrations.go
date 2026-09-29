// Package migrations holds the schema's history. Each migration is a Go file
// named <timestamp>_<what it does>.go — bun reads the name off the file itself —
// and builds its tables from the types in internal/model, so a column is
// declared once and the DDL follows from it.
//
// Nothing here gives a timestamp a SQL DEFAULT. Every time value is written by
// the server, in one format, through bun: a column that two writers fill in two
// formats compares and sorts wrongly, and the sliding session expiry is a
// comparison on exactly such a column.
package migrations

import (
	"context"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/migrate"
)

// Migrations is the set the server applies at startup, in name order.
var Migrations = migrate.NewMigrations()

// create makes one table per model, in the order given, adding the foreign keys
// listed for it. The keys are written out rather than derived from the model's
// relations because ON DELETE is a decision about the data, not about the shape
// of it: which rows are part of a user and die with them, and which are not.
func create(ctx context.Context, db *bun.DB, models []any, foreignKeys map[any]string) error {
	for _, model := range models {
		query := db.NewCreateTable().Model(model)
		if foreignKey, ok := foreignKeys[model]; ok {
			query = query.ForeignKey(foreignKey)
		}
		if _, err := query.Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

// drop removes tables back to front, so a table is never dropped while another
// one still references it.
func drop(ctx context.Context, db *bun.DB, models []any) error {
	for index := len(models) - 1; index >= 0; index-- {
		if _, err := db.NewDropTable().Model(models[index]).IfExists().Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}
