package server

import (
	"context"
	"errors"
	"log"
	"math"
	"slices"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"control/internal/schema"
)

// errorPresenter states every failure in the terms the schema defines: a code
// from ErrorCode, and — when an argument is at fault — which argument and why.
//
// Three kinds of error arrive here, and they are told apart by what they carry
// rather than by where they came from:
//
//   - a schema.CodedError, which already says what it is;
//   - an error raised while coercing an argument, which gqlgen tags with a path
//     deeper than the field being resolved;
//   - anything else, which is a bug in this server and is reported as INTERNAL
//     with its details kept out of the response and put in the log instead.
func errorPresenter(ctx context.Context, err error) *gqlerror.Error {
	presented := graphql.DefaultErrorPresenter(ctx, err)
	if presented.Extensions == nil {
		presented.Extensions = map[string]any{}
	}

	var codedError *schema.CodedError
	if errors.As(err, &codedError) {
		presented.Extensions["code"] = codedError.Code
		if codedError.RetryAfter > 0 {
			presented.Extensions["retryAfterSeconds"] = retryAfterSeconds(codedError.RetryAfter)
		}
		if len(codedError.InputPath) > 0 {
			presented.Extensions["inputPath"] = codedError.InputPath
			presented.Extensions["reason"] = codedError.Reason
			presented.Path = trimInputPath(presented.Path, codedError.InputPath)
		}
		return presented
	}

	if inputPath := argumentPath(ctx, presented); len(inputPath) > 0 {
		presented.Extensions["code"] = schema.ErrorCodeInvalidInput
		presented.Extensions["inputPath"] = inputPath
		presented.Extensions["reason"] = schema.InvalidInputReasonMalformed
		presented.Path = trimInputPath(presented.Path, inputPath)
		return presented
	}

	if code, ok := presented.Extensions["code"].(string); ok && clientFault(code) {
		// gqlgen's own parse and validation failures: the operation never
		// reached a resolver, and the client is the one who can fix it.
		presented.Extensions["code"] = schema.ErrorCodeInvalidInput
		return presented
	}

	log.Printf("internal error: %v", err)
	presented.Message = "internal server error"
	presented.Extensions["code"] = schema.ErrorCodeInternal
	return presented
}

// retryAfterSeconds rounds a wait up to whole seconds, as Retry-After does: a
// client that waits the rounded-down figure would arrive just too early.
func retryAfterSeconds(wait time.Duration) int {
	return int(math.Ceil(wait.Seconds()))
}

// clientFault lists the gqlgen error codes that mean the request was wrong
// rather than the server. They are gqlgen's strings, not the schema's, which is
// exactly why they are translated here instead of being passed through.
func clientFault(code string) bool {
	return code == "GRAPHQL_PARSE_FAILED" || code == "GRAPHQL_VALIDATION_FAILED"
}

// argumentPath reports the path of the argument an error is about, relative to
// the field, or nothing if the error is not about an argument.
//
// The test is positional: gqlgen pushes a path element for every argument and
// every input-object field it coerces, on top of the path of the field itself.
// So an error whose path runs deeper than the field's happened while reading the
// call, and an error whose path stops at the field happened while answering it.
func argumentPath(ctx context.Context, presented *gqlerror.Error) []string {
	fieldContext := graphql.GetFieldContext(ctx)
	if fieldContext == nil {
		return nil
	}
	fieldPath := fieldContext.Path()
	if len(presented.Path) <= len(fieldPath) {
		return nil
	}
	return pathNames(presented.Path[len(fieldPath):])
}

// trimInputPath cuts the argument's path off the error's path, so that path
// names the field that failed and inputPath names what in the call it failed on
// — which is the division the schema documents.
func trimInputPath(path ast.Path, inputPath []string) ast.Path {
	if len(inputPath) > len(path) {
		return path
	}
	tail := pathNames(path[len(path)-len(inputPath):])
	if !slices.Equal(tail, inputPath) {
		// The error was raised by a resolver that named the argument itself, so
		// the path never had the argument on it to begin with.
		return path
	}
	return path[:len(path)-len(inputPath)]
}

// pathNames keeps the named steps of a path and drops list indices, which
// cannot name an argument.
func pathNames(path ast.Path) []string {
	names := make([]string, 0, len(path))
	for _, step := range path {
		if name, ok := step.(ast.PathName); ok {
			names = append(names, string(name))
		}
	}
	return names
}
