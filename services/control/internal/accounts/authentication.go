package accounts

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"control/internal/database"
	"control/internal/identity"
	"control/internal/model"
	passwordreset "control/internal/password_reset"
	"control/internal/schema"
	"control/internal/session"
)

// setup creates the first account. It is the only way one comes into existence
// without an existing one, so it is guarded to run exactly once.
func (s *Service) Setup(ctx context.Context, input schema.SetupInput) (*schema.Viewer, error) {
	if err := checkPassword(input.Password, []string{"input", "password"}, input.Username, input.DisplayName); err != nil {
		return nil, err
	}
	passwordHash, err := identity.HashPassword(input.Password)
	if err != nil {
		return nil, err
	}

	now := nowUTC()
	root := &model.User{
		// The root account is row one, and the primary key is what makes that a
		// rule rather than a hope: two concurrent setups cannot both insert it,
		// so the loser is told what a latecomer would be told anyway.
		ID:            1,
		Username:      input.Username,
		DisplayName:   input.DisplayName,
		PasswordHash:  passwordHash,
		Rank:          rootRank,
		PasswordSetAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	err = s.database.RunInTx(ctx, nil, func(ctx context.Context, transaction bun.Tx) error {
		taken, err := transaction.NewSelect().Model((*model.User)(nil)).Exists(ctx)
		if err != nil {
			return err
		}
		if taken {
			return errSetupAlreadyDone
		}
		if _, err := transaction.NewInsert().Model(root).Exec(ctx); err != nil {
			return err
		}
		_, err = transaction.NewInsert().
			Model(&model.UserRole{UserID: root.ID, Role: string(schema.RoleAdmin)}).
			Exec(ctx)
		return err
	})
	if errors.Is(err, errSetupAlreadyDone) || database.IsUniqueViolation(err) {
		// Not an error to the client: setup is the question "is this server
		// still unclaimed", and the answer is no.
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.signIn(ctx, root.ID, sessionTTL(input.SessionTTLSeconds))
}

// errSetupAlreadyDone travels out of the transaction and no further: the field
// answers with null rather than with a failure.
var errSetupAlreadyDone = errors.New("setup has already been done")

// login exchanges a username and password for a session.
func (s *Service) Login(ctx context.Context, input schema.LoginInput) (*schema.Viewer, error) {
	var user model.User
	err := s.database.NewSelect().
		Model(&user).
		Column("id", "password_hash").
		Where("username = ?", input.Username).
		Scan(ctx)
	if err != nil && !isNoRows(err) {
		return nil, err
	}
	// An unknown user leaves the hash empty, and so does an account whose
	// password was never set. Neither verifies, and both are checked all the
	// same, so neither answers any differently from a wrong password.
	if err := s.provePassword(ctx, input.Username, input.Password, user.PasswordHash); err != nil {
		return nil, err
	}
	return s.signIn(ctx, user.ID, sessionTTL(input.SessionTTLSeconds))
}

// signIn issues the session the sign-in paths share and answers with the viewer
// it created.
func (s *Service) signIn(ctx context.Context, userID int64, ttl time.Duration) (*schema.Viewer, error) {
	issued, err := s.startSession(ctx, userID, ttl)
	if err != nil {
		return nil, err
	}
	s.publishSessions(userID)
	return s.viewer(ctx, identity.Principal{
		UserID:          userID,
		SessionID:       issued.ID,
		StepUpExpiresAt: issued.StepUpExpiresAt,
	})
}

// logout ends the current session and clears its cookie.
func (s *Service) Logout(ctx context.Context) (*schema.Void, error) {
	principal, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if err := session.End(ctx, s.database, principal.SessionID); err != nil {
		return nil, err
	}
	identity.ClearSessionCookie(ctx)
	s.publishSessionEnded(principal.UserID, principal.SessionID)
	return done, nil
}

// stepUp raises the current session by proving the password again.
func (s *Service) StepUp(ctx context.Context, input schema.StepUpInput) (*schema.Viewer, error) {
	principal, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.verifyPassword(ctx, principal.UserID, input.Password); err != nil {
		return nil, err
	}
	stepUpExpiresAt, err := session.Raise(ctx, s.database, principal.SessionID)
	if err != nil {
		return nil, err
	}
	principal.StepUpExpiresAt = stepUpExpiresAt
	s.publishViewer(principal.UserID)
	return s.viewer(ctx, principal)
}

// changePassword replaces the caller's own password, which ends every session
// but the one asking: a password change is how somebody reacts to a device they
// no longer trust.
func (s *Service) ChangePassword(ctx context.Context, input schema.ChangePasswordInput) (*schema.Viewer, error) {
	principal, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.verifyPassword(ctx, principal.UserID, input.CurrentPassword); err != nil {
		return nil, err
	}
	user, err := s.loadUser(ctx, principal.UserID)
	if err != nil {
		return nil, err
	}
	if err := checkPassword(input.NewPassword, []string{"input", "newPassword"}, user.Username, user.DisplayName); err != nil {
		return nil, err
	}
	if err := s.writePassword(ctx, principal.UserID, input.NewPassword); err != nil {
		return nil, err
	}

	ended, err := session.EndOthers(ctx, s.database, principal.UserID, principal.SessionID)
	if err != nil {
		return nil, err
	}
	for _, sessionID := range ended {
		s.publishSessionEnded(principal.UserID, sessionID)
	}
	// The password was just proved, so the session is raised: a caller who
	// changed their password is not asked for it again a moment later.
	stepUpExpiresAt, err := session.Raise(ctx, s.database, principal.SessionID)
	if err != nil {
		return nil, err
	}
	principal.StepUpExpiresAt = stepUpExpiresAt
	s.publishSessions(principal.UserID)
	return s.viewer(ctx, principal)
}

// updateProfile patches the caller's own profile. Absent fields are left alone,
// which is what makes the input safe to grow: an avatar or a description can be
// added later without every client having to send the fields it does not touch.
func (s *Service) UpdateProfile(ctx context.Context, input schema.UpdateProfileInput) (*schema.Viewer, error) {
	principal, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if input.DisplayName == nil {
		return nil, schema.InvalidInput(schema.InvalidInputReasonMalformed, []string{"input"},
			"at least one field is required")
	}
	if _, err := s.database.NewUpdate().
		Model((*model.User)(nil)).
		Set("display_name = ?", *input.DisplayName).
		Set("updated_at = ?", nowUTC()).
		Where("id = ?", principal.UserID).
		Exec(ctx); err != nil {
		return nil, err
	}
	s.publishViewer(principal.UserID)
	s.publishUsers()
	return s.viewer(ctx, principal)
}

// requestPasswordReset issues a secret for whoever owns the username, and says
// nothing about whether anybody does. The secret goes to the server console
// because this server has no channel of its own to deliver it on; an operator
// reads it there and hands it over the way they already hand over access.
func (s *Service) RequestPasswordReset(ctx context.Context, username string) (*schema.Void, error) {
	if err := s.admitResetRequest(ctx, username); err != nil {
		return nil, err
	}
	var user model.User
	err := s.database.NewSelect().
		Model(&user).
		Column("id", "password_reset_issued_at").
		Where("username = ?", username).
		Scan(ctx)
	if isNoRows(err) {
		return done, nil
	}
	if err != nil {
		return nil, err
	}
	// A secret issued moments ago is left in place, and the answer is the
	// usual one. Only an account can have such a secret, so saying anything
	// else would say the account exists; and replacing it would let anybody
	// keep cancelling the owner's link by asking again.
	if !user.PasswordResetIssuedAt.IsZero() && time.Since(user.PasswordResetIssuedAt) < s.limits.resetInterval {
		return done, nil
	}
	if err := s.issuePasswordReset(ctx, user.ID, username); err != nil {
		return nil, err
	}
	return done, nil
}

// issuePasswordReset mints a secret and prints the link that redeems it.
func (s *Service) issuePasswordReset(ctx context.Context, userID int64, username string) error {
	secret, expiresAt, err := passwordreset.Issue(ctx, s.database, userID)
	if err != nil {
		return err
	}
	log.Printf("password reset for %q, valid until %s: %s",
		username, expiresAt.Format(time.RFC3339), s.passwordResetLink(secret))
	return nil
}

// passwordResetLink is where the UI redeems a secret: its /set-password page,
// with the secret in the fragment. A fragment never leaves the browser, so the
// secret stays out of every request log between the user and this server.
func (s *Service) passwordResetLink(secret string) string {
	return strings.TrimSuffix(s.config.PublicURL, "/") + "/set-password#" + secret
}

// passwordResetInfo tells a client holding a secret whose account it opens, so
// the form can name the account before a password is typed into it. A secret
// that is not valid is not an error — the link is simply no longer a link.
func (s *Service) PasswordResetInfo(ctx context.Context, secret string) (*schema.PasswordResetInfo, error) {
	info, err := passwordreset.Verify(ctx, s.database, secret)
	if errors.Is(err, passwordreset.ErrInvalid) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &schema.PasswordResetInfo{Username: info.Username, ExpiresAt: info.ExpiresAt}, nil
}

// setPassword redeems a reset secret. Every session the account had ends: the
// secret is used precisely when nobody is sure who else is holding the account.
func (s *Service) SetPassword(ctx context.Context, secret string, input schema.SetPasswordInput) (*schema.Viewer, error) {
	info, err := passwordreset.Verify(ctx, s.database, secret)
	if errors.Is(err, passwordreset.ErrInvalid) {
		return nil, schema.InvalidInput(schema.InvalidInputReasonTargetMissing, []string{"secret"},
			"this password reset secret is no longer valid")
	}
	if err != nil {
		return nil, err
	}
	if err := checkPassword(input.Password, []string{"input", "password"}, info.Username); err != nil {
		return nil, err
	}
	if err := s.writePassword(ctx, info.UserID, input.Password); err != nil {
		return nil, err
	}
	// Clearing is belt and braces: the new password hash already invalidates
	// the stored one. It also drops the issue time with it.
	if err := passwordreset.Clear(ctx, s.database, info.UserID); err != nil {
		return nil, err
	}

	ended, err := session.EndOthers(ctx, s.database, info.UserID, 0)
	if err != nil {
		return nil, err
	}
	for _, sessionID := range ended {
		s.publishSessionEnded(info.UserID, sessionID)
	}
	return s.signIn(ctx, info.UserID, sessionTTL(input.SessionTTLSeconds))
}

// writePassword stores a new password hash and records when it was set.
func (s *Service) writePassword(ctx context.Context, userID int64, password string) error {
	passwordHash, err := identity.HashPassword(password)
	if err != nil {
		return err
	}
	now := nowUTC()
	_, err = s.database.NewUpdate().
		Model((*model.User)(nil)).
		Set("password_hash = ?", passwordHash).
		Set("password_set_at = ?", now).
		Set("updated_at = ?", now).
		Where("id = ?", userID).
		Exec(ctx)
	return err
}

// verifyPassword checks a password against the one a user currently has.
func (s *Service) verifyPassword(ctx context.Context, userID int64, password string) error {
	var user model.User
	if err := s.database.NewSelect().
		Model(&user).
		Column("username", "password_hash").
		Where("id = ?", userID).
		Scan(ctx); err != nil {
		return err
	}
	return s.provePassword(ctx, user.Username, password, user.PasswordHash)
}

// checkPassword applies the one password rule the schema cannot state. Length is
// declared as a @constraint and enforced with the rest of them; being easy to
// guess is not expressible there, so it is enforced here and reported in the
// same shape, on the same path.
func checkPassword(password string, path []string, personal ...string) error {
	if identity.Guessable(password, personal...) {
		return schema.InvalidInput(schema.InvalidInputReasonTooGuessable, path,
			"this password is too easy to guess")
	}
	return nil
}
