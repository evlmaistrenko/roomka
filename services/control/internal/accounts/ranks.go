package accounts

import (
	"context"

	"control/internal/identity"
	"control/internal/model"
	"control/internal/schema"
)

// The half of access control that is not in the schema.
//
// @auth says which permission a field needs; rank says who a holder of that
// permission may use it on. The two are separate axes on purpose: a permission
// is what you can do, a rank is who you can do it to, and collapsing them would
// mean either one role per level of seniority or an administrator who can demote
// the person who appointed them.
//
// The rule is a strict inequality in both directions — a user acts only on lower
// ranks, and assigns no rank but a lower one — and everything else follows from
// it without a rule of its own:
//
//   - Nobody can act on themselves, since a rank is not below itself. Managing
//     your own account goes through the fields that are about you (updateProfile,
//     changePassword), which is the only place it belongs.
//   - Nobody can reach the root user, who sits at a rank the schema's own
//     @constraint(max: 99) puts out of reach. So "the root user cannot be
//     demoted, deleted or reset" needs no check anywhere.
//   - Nobody can appoint a peer. An administrator builds a hierarchy below
//     themselves, never beside themselves.

// rankOf reads a user's rank.
func (s *Service) rankOf(ctx context.Context, userID int64) (int, error) {
	var rank int
	err := s.database.NewSelect().
		Model((*model.User)(nil)).
		Column("rank").
		Where("id = ?", userID).
		Scan(ctx, &rank)
	return rank, err
}

// target resolves the userId argument of a user-management field and checks that
// the caller outranks whoever it names.
//
// A user who does not exist and a user who is out of reach give the same two
// answers they would give separately, in the same order they would come in
// anyway: the id is checked first because the caller supplied it, and an
// out-of-reach user is ACCESS_DENIED rather than TARGET_MISSING because denying
// what exists is the honest answer to someone who may list every user.
func (s *Service) target(ctx context.Context, principal identity.Principal, userID string) (int64, error) {
	id, ok := parseID(userID)
	if !ok {
		return 0, errTargetMissing
	}
	targetRank, err := s.rankOf(ctx, id)
	if isNoRows(err) {
		return 0, errTargetMissing
	}
	if err != nil {
		return 0, err
	}
	callerRank, err := s.rankOf(ctx, principal.UserID)
	if err != nil {
		return 0, err
	}
	if callerRank <= targetRank {
		return 0, schema.ErrAccessDenied
	}
	return id, nil
}

// assigns checks that the caller may hand out the rank they are asking for.
func (s *Service) assigns(ctx context.Context, principal identity.Principal, rank int) error {
	callerRank, err := s.rankOf(ctx, principal.UserID)
	if err != nil {
		return err
	}
	if callerRank <= rank {
		return schema.ErrAccessDenied
	}
	return nil
}

// errTargetMissing is what every stale id gets: the row the client is looking at
// is not there any more, or never was.
var errTargetMissing = schema.InvalidInput(schema.InvalidInputReasonTargetMissing,
	[]string{"userId"}, "no such user")
