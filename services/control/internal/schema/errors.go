package schema

import "time"

// CodedError is a failure stated in the terms the schema defines: a code from
// the ErrorCode enum and, when an argument is at fault, which argument and why.
// The error presenter (see internal/server) copies all three into extensions, so
// clients branch on the code rather than on the message text.
//
// It lives here, with the enums it speaks in, because both sides of the server
// raise one: the domain when it refuses to do something, and the directives when
// they refuse to let it be asked.
//
// Code and Reason are the generated enum types rather than plain strings on
// purpose: the set of codes then has exactly one definition — the schema — and a
// code that is not declared there fails to compile.
type CodedError struct {
	Code    ErrorCode
	Message string
	// InputPath names the offending argument, outermost first, relative to the
	// field. Empty for failures that are not about an argument.
	InputPath []string
	// Reason is set exactly when Code is INVALID_INPUT.
	Reason InvalidInputReason
	// RetryAfter is set exactly when Code is RATE_LIMITED.
	RetryAfter time.Duration
}

func (e *CodedError) Error() string { return e.Message }

// Coded builds a failure that is not about an argument.
func Coded(code ErrorCode, message string) error {
	return &CodedError{Code: code, Message: message}
}

// InvalidInput builds an argument failure. The path is relative to the field, so
// a top-level argument is one element and a field of an input object is two.
func InvalidInput(reason InvalidInputReason, path []string, message string) error {
	return &CodedError{
		Code:      ErrorCodeInvalidInput,
		Message:   message,
		InputPath: path,
		Reason:    reason,
	}
}

// RateLimited builds the failure for an attempt made too soon. The message is
// the same whatever was limited: a different one per limit would say which
// counter a caller has run out of, which is a hint at what they were probing.
func RateLimited(retryAfter time.Duration) error {
	return &CodedError{
		Code:       ErrorCodeRateLimited,
		Message:    "too many attempts, try again later",
		RetryAfter: retryAfter,
	}
}

// The failures that carry no detail beyond their code. They are values rather
// than constructors because every caller phrases them identically: saying more
// would describe what the caller is not allowed to know.
var (
	ErrUnauthenticated = Coded(ErrorCodeUnauthenticated, "authentication required")
	ErrAccessDenied    = Coded(ErrorCodeAccessDenied, "access denied")
	ErrStepUpRequired  = Coded(ErrorCodeStepUpRequired, "this operation requires a recently proved password")
	// ErrInvalidCredentials answers login and stepUp. It is deliberately one
	// message for "no such user" and "wrong password": distinguishing them turns
	// the login form into a list of who has an account here.
	ErrInvalidCredentials = Coded(ErrorCodeUnauthenticated, "invalid username or password")
)
