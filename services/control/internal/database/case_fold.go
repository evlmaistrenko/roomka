package database

import (
	"database/sql/driver"
	"errors"
	"strings"

	"modernc.org/sqlite"
)

// FoldCase is the name of a Unicode-aware lower() available to every query.
//
// sqlite's own lower() and its case-insensitive LIKE fold ASCII only, so a
// search for "Женя" would match nothing a person typed differently. Go's
// strings.ToLower folds the whole of Unicode, so registering it as a function
// keeps case-insensitive search a SQL-level operation — which is what lets it
// stay inside the same query as the paging and the limit.
const FoldCase = "foldCase"

func init() {
	sqlite.MustRegisterDeterministicScalarFunction(FoldCase, 1,
		func(_ *sqlite.FunctionContext, arguments []driver.Value) (driver.Value, error) {
			text, ok := arguments[0].(string)
			if !ok {
				// NULL in, NULL out, like every other sqlite scalar function.
				return nil, nil
			}
			return strings.ToLower(text), nil
		})
}

// sqliteConstraintUnique is SQLITE_CONSTRAINT_UNIQUE, the extended result code
// for a unique index rejecting a row.
const sqliteConstraintUnique = 2067

// IsUniqueViolation reports whether err is a unique constraint failure. Callers
// use it to turn a race — two clients claiming one username — into the same
// answer the loser would have got from a prior check.
func IsUniqueViolation(err error) bool {
	var sqliteError *sqlite.Error
	return errors.As(err, &sqliteError) && sqliteError.Code() == sqliteConstraintUnique
}
