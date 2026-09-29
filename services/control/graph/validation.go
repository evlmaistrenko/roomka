package graph

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"unicode/utf8"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"

	"control/internal/schema"
)

// Input validation: everything @constraint declares, and nothing else. It is a
// directive like @auth and has nothing else in common with it — one decides
// whether a call is allowed, the other whether it is well formed — so the two
// are neighbours in the schema and strangers here.

// directiveConstraint implements @constraint. gqlgen runs it around the
// unmarshaling of the value it guards, so next returns the parsed Go value and
// this only has to judge it. Validation stops at the first failing argument,
// which is what returning an error here does.
func directiveConstraint(ctx context.Context, _ any, next graphql.Resolver, minLength, maxLength *int, pattern *string, minimum, maximum *int) (any, error) {
	value, err := next(ctx)
	if err != nil || value == nil {
		return value, err
	}
	switch typed := value.(type) {
	case string:
		return value, checkText(ctx, typed, minLength, maxLength, pattern)
	case *string:
		if typed == nil {
			return value, nil
		}
		return value, checkText(ctx, *typed, minLength, maxLength, pattern)
	case int:
		return value, checkNumber(ctx, typed, minimum, maximum)
	case *int:
		if typed == nil {
			return value, nil
		}
		return value, checkNumber(ctx, *typed, minimum, maximum)
	default:
		// A constraint that silently does nothing is worse than one that fails
		// loudly: it would read as enforced everywhere the schema declares it.
		return nil, schema.Coded(schema.ErrorCodeInternal, fmt.Sprintf("@constraint cannot be applied to %T", value))
	}
}

func checkText(ctx context.Context, value string, minLength, maxLength *int, pattern *string) error {
	// Counted in characters rather than bytes: a length rule the client can also
	// apply has to mean the same thing on both sides, and a client counts what a
	// person typed.
	length := utf8.RuneCountInString(value)
	if minLength != nil && length < *minLength {
		return constraintError(ctx, schema.InvalidInputReasonTooShort,
			fmt.Sprintf("must be at least %d characters", *minLength))
	}
	if maxLength != nil && length > *maxLength {
		return constraintError(ctx, schema.InvalidInputReasonTooLong,
			fmt.Sprintf("must be at most %d characters", *maxLength))
	}
	if pattern != nil {
		expression, err := compilePattern(*pattern)
		if err != nil {
			return schema.Coded(schema.ErrorCodeInternal, fmt.Sprintf("@constraint pattern %q does not compile: %v", *pattern, err))
		}
		if !expression.MatchString(value) {
			return constraintError(ctx, schema.InvalidInputReasonMalformed,
				fmt.Sprintf("must match %s", *pattern))
		}
	}
	return nil
}

func checkNumber(ctx context.Context, value int, minimum, maximum *int) error {
	if minimum != nil && value < *minimum {
		return constraintError(ctx, schema.InvalidInputReasonOutOfRange,
			fmt.Sprintf("must be at least %d", *minimum))
	}
	if maximum != nil && value > *maximum {
		return constraintError(ctx, schema.InvalidInputReasonOutOfRange,
			fmt.Sprintf("must be at most %d", *maximum))
	}
	return nil
}

// constraintError builds the failure with the path of the value being
// unmarshaled, which is where the argument's name comes from.
func constraintError(ctx context.Context, reason schema.InvalidInputReason, message string) error {
	path := inputPath(ctx)
	name := "input"
	if len(path) > 0 {
		name = path[len(path)-1]
	}
	return schema.InvalidInput(reason, path, name+" "+message)
}

// inputPath is the path of the value under validation, relative to the field
// that carries it. gqlgen pushes one path element per argument and per input
// object field, on top of the field's own path, so dropping that prefix leaves
// exactly what the schema calls inputPath.
func inputPath(ctx context.Context) []string {
	full := graphql.GetPath(ctx)
	fieldContext := graphql.GetFieldContext(ctx)
	if fieldContext == nil {
		return pathNames(full)
	}
	prefix := len(fieldContext.Path())
	if prefix > len(full) {
		return nil
	}
	return pathNames(full[prefix:])
}

// pathNames keeps the named steps of a path. List indices cannot name an
// argument, so they are dropped rather than rendered as numbers.
func pathNames(path ast.Path) []string {
	names := make([]string, 0, len(path))
	for _, step := range path {
		if name, ok := step.(ast.PathName); ok {
			names = append(names, string(name))
		}
	}
	return names
}

// patterns caches compiled @constraint patterns. The schema has a fixed set of
// them, so the cache is bounded by the schema rather than by traffic.
var patterns sync.Map

func compilePattern(pattern string) (*regexp.Regexp, error) {
	if cached, ok := patterns.Load(pattern); ok {
		return cached.(*regexp.Regexp), nil
	}
	expression, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	patterns.Store(pattern, expression)
	return expression, nil
}
