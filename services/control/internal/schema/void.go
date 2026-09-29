package schema

import (
	"errors"
	"io"

	"github.com/99designs/gqlgen/graphql"
)

// Void is the schema's Void scalar: the result of a field that has no result
// beyond having succeeded. It always marshals to null, and no input position
// accepts it, so the only value a client can legitimately send is null.
type Void struct{}

func (Void) MarshalGQL(writer io.Writer) {
	_, _ = writer.Write([]byte("null"))
}

func (*Void) UnmarshalGQL(value any) error {
	if value != nil {
		return errors.New("Void accepts null only")
	}
	return nil
}

// compile-time guard: Void has to satisfy both halves of a gqlgen scalar.
var (
	_ graphql.Marshaler   = Void{}
	_ graphql.Unmarshaler = (*Void)(nil)
)
