package accounts

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"control/internal/database"
	"control/internal/model"
	"control/internal/schema"
	"control/internal/session"
)

// users answers the user list, one page at a time.
//
// Paging is keyset rather than offset: the cursor names the last row of the page
// by its sort value and id, so a page is "the rows after this one" rather than
// "rows 50 to 100". Rows created or deleted while a client pages therefore
// cannot make it skip or repeat one — which is the same property the schema
// promises by calling the cursor stable.
func (s *Service) Users(ctx context.Context, filter *schema.UserFilterInput, sortBy *schema.UserSort, after *string, limit int) (*schema.UserPage, error) {
	sort := userSort(sortBy)

	totalCount, err := s.filterUsers(s.database.NewSelect().Model((*model.User)(nil)), filter).Count(ctx)
	if err != nil {
		return nil, err
	}

	var stored []model.User
	query := s.filterUsers(s.database.NewSelect().Model(&stored), filter).
		// One query each for the roles and the permissions of the whole page,
		// rather than two per user.
		Relation("Roles").
		Relation("Permissions")
	if after != nil {
		query, err = sort.seek(query, *after, pageFingerprint(sortBy, filter))
		if err != nil {
			return nil, err
		}
	}
	// One row more than asked for: whether it exists is exactly hasNextPage, and
	// it costs nothing compared to counting the rest.
	if err := query.Order(sort.orderBy()...).Limit(limit + 1).Scan(ctx); err != nil {
		return nil, err
	}

	hasNextPage := len(stored) > limit
	if hasNextPage {
		stored = stored[:limit]
	}
	items := make([]*schema.User, 0, len(stored))
	for _, one := range stored {
		items = append(items, s.toUser(one))
	}

	page := &schema.UserPage{Items: items, TotalCount: totalCount, PageInfo: &schema.PageInfo{HasNextPage: hasNextPage}}
	if last := len(stored) - 1; last >= 0 {
		cursor := encodeCursor(pageFingerprint(sortBy, filter), sort.cursorValue(stored[last]), stored[last].ID)
		page.PageInfo.EndCursor = &cursor
	}
	return page, nil
}

// createUser makes an account nobody has signed into yet: no password, and a
// reset secret to claim it with. There is no separate invitation to accept,
// because the row this creates is the whole of what accepting one would make.
func (s *Service) CreateUser(ctx context.Context, input schema.CreateUserInput) (*schema.User, error) {
	principal, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.assigns(ctx, principal, input.Rank); err != nil {
		return nil, err
	}

	now := nowUTC()
	created := &model.User{
		Username:    input.Username,
		DisplayName: input.DisplayName,
		Rank:        input.Rank,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	err = s.database.RunInTx(ctx, nil, func(ctx context.Context, transaction bun.Tx) error {
		if _, err := transaction.NewInsert().Model(created).Exec(ctx); err != nil {
			return err
		}
		if err := writeRoles(ctx, transaction, created.ID, input.Roles); err != nil {
			return err
		}
		return writePermissions(ctx, transaction, created.ID, input.Permissions)
	})
	if database.IsUniqueViolation(err) {
		return nil, schema.InvalidInput(schema.InvalidInputReasonTaken, []string{"input", "username"},
			"that username is taken")
	}
	if err != nil {
		return nil, err
	}

	if err := s.issuePasswordReset(ctx, created.ID, input.Username); err != nil {
		return nil, err
	}
	s.publishUsers()
	return s.loadUser(ctx, created.ID)
}

// deleteUser removes an account and everything that hangs off it — sessions,
// preferences, roles, permissions — which the foreign keys cascade.
func (s *Service) DeleteUser(ctx context.Context, userID string) (*schema.Void, error) {
	principal, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	id, err := s.target(ctx, principal, userID)
	if err != nil {
		return nil, err
	}
	if _, err := s.database.NewDelete().
		Model((*model.User)(nil)).
		Where("id = ?", id).
		Exec(ctx); err != nil {
		return nil, err
	}
	s.publishAccessChanged(id)
	return done, nil
}

// resetUserCredentials takes an account away from whoever is currently using it:
// the password goes at once, every session ends, and a fresh secret is printed
// for handing back to its owner. It is the answer to a compromised account, so
// it does not wait for the owner to ask.
func (s *Service) ResetUserCredentials(ctx context.Context, userID string) (*schema.User, error) {
	principal, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	id, err := s.target(ctx, principal, userID)
	if err != nil {
		return nil, err
	}

	var username string
	if err := s.database.NewSelect().
		Model((*model.User)(nil)).
		Column("username").
		Where("id = ?", id).
		Scan(ctx, &username); err != nil {
		return nil, err
	}
	if _, err := s.database.NewUpdate().
		Model((*model.User)(nil)).
		Set("password_hash = ?", "").
		Set("password_set_at = NULL").
		Set("updated_at = ?", nowUTC()).
		Where("id = ?", id).
		Exec(ctx); err != nil {
		return nil, err
	}
	ended, err := session.EndOthers(ctx, s.database, id, 0)
	if err != nil {
		return nil, err
	}
	for _, sessionID := range ended {
		s.publishSessionEnded(id, sessionID)
	}
	// Issued after the password is cleared, so the secret is signed against the
	// empty hash it will be redeemed against.
	if err := s.issuePasswordReset(ctx, id, username); err != nil {
		return nil, err
	}
	s.publishAccessChanged(id)
	return s.loadUser(ctx, id)
}

// setUserRoles replaces the roles a user holds.
func (s *Service) SetUserRoles(ctx context.Context, userID string, input schema.SetUserRolesInput) (*schema.User, error) {
	return s.rewriteGrants(ctx, userID, func(ctx context.Context, transaction bun.IDB, id int64) error {
		return writeRoles(ctx, transaction, id, input.Roles)
	})
}

// setUserPermissions replaces the permissions a user holds outside any role.
func (s *Service) SetUserPermissions(ctx context.Context, userID string, input schema.SetUserPermissionsInput) (*schema.User, error) {
	return s.rewriteGrants(ctx, userID, func(ctx context.Context, transaction bun.IDB, id int64) error {
		return writePermissions(ctx, transaction, id, input.Permissions)
	})
}

// setUserRank moves a user within the hierarchy, to a rank the caller may hand
// out — which, being strictly below their own, can never be used against them.
func (s *Service) SetUserRank(ctx context.Context, userID string, input schema.SetUserRankInput) (*schema.User, error) {
	principal, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.assigns(ctx, principal, input.Rank); err != nil {
		return nil, err
	}
	return s.rewriteGrants(ctx, userID, func(ctx context.Context, transaction bun.IDB, id int64) error {
		_, err := transaction.NewUpdate().
			Model((*model.User)(nil)).
			Set("rank = ?", input.Rank).
			Set("updated_at = ?", nowUTC()).
			Where("id = ?", id).
			Exec(ctx)
		return err
	})
}

// rewriteGrants is the shape the three "set what this user may do" mutations
// share: resolve the target under the rank rule, change it in one transaction,
// then tell everyone watching that this user's access moved.
func (s *Service) rewriteGrants(ctx context.Context, userID string, write func(context.Context, bun.IDB, int64) error) (*schema.User, error) {
	principal, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	id, err := s.target(ctx, principal, userID)
	if err != nil {
		return nil, err
	}
	if err := s.database.RunInTx(ctx, nil, func(ctx context.Context, transaction bun.Tx) error {
		return write(ctx, transaction, id)
	}); err != nil {
		return nil, err
	}

	s.publishAccessChanged(id)
	return s.loadUser(ctx, id)
}

// writeRoles replaces a user's roles with exactly the ones given.
func writeRoles(ctx context.Context, database bun.IDB, userID int64, roles []schema.Role) error {
	if _, err := database.NewDelete().
		Model((*model.UserRole)(nil)).
		Where("user_id = ?", userID).
		Exec(ctx); err != nil {
		return err
	}
	if len(roles) == 0 {
		return nil
	}
	rows := make([]model.UserRole, 0, len(roles))
	for _, role := range roles {
		rows = append(rows, model.UserRole{UserID: userID, Role: string(role)})
	}
	// Ignore, because a client listing the same role twice is asking for it to
	// be held, not for an error.
	_, err := database.NewInsert().Model(&rows).Ignore().Exec(ctx)
	return err
}

// writePermissions replaces a user's direct permissions with the ones given.
func writePermissions(ctx context.Context, database bun.IDB, userID int64, permissions []schema.Permission) error {
	if _, err := database.NewDelete().
		Model((*model.UserPermission)(nil)).
		Where("user_id = ?", userID).
		Exec(ctx); err != nil {
		return err
	}
	if len(permissions) == 0 {
		return nil
	}
	rows := make([]model.UserPermission, 0, len(permissions))
	for _, permission := range permissions {
		rows = append(rows, model.UserPermission{UserID: userID, Permission: string(permission)})
	}
	_, err := database.NewInsert().Model(&rows).Ignore().Exec(ctx)
	return err
}

// filterUsers narrows a user query. Search is case-insensitive over the whole of
// Unicode, which needs foldCase on both sides — see internal/database.
func (s *Service) filterUsers(query *bun.SelectQuery, filter *schema.UserFilterInput) *bun.SelectQuery {
	if filter == nil {
		return query
	}
	if filter.Search != nil {
		pattern := "%" + escapeLike(*filter.Search) + "%"
		query = query.Where(
			`(foldCase(username) LIKE foldCase(?) ESCAPE '\' OR foldCase(display_name) LIKE foldCase(?) ESCAPE '\')`,
			pattern, pattern)
	}
	if filter.Role != nil {
		query = query.Where("EXISTS (?)", s.database.NewSelect().
			Model((*model.UserRole)(nil)).
			Column("user_id").
			Where("user_role.user_id = user.id").
			Where("user_role.role = ?", string(*filter.Role)))
	}
	if filter.HasPassword != nil {
		if *filter.HasPassword {
			query = query.Where("password_set_at IS NOT NULL")
		} else {
			query = query.Where("password_set_at IS NULL")
		}
	}
	if filter.CreatedAfter != nil {
		query = query.Where("created_at > ?", *filter.CreatedAfter)
	}
	if filter.CreatedBefore != nil {
		query = query.Where("created_at < ?", *filter.CreatedBefore)
	}
	return query
}

// escapeLike neutralises the wildcards in a user-supplied LIKE pattern, so a
// search for "%" means the character rather than "everything".
func escapeLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}

// usersSort is one of the schema's UserSort values, as the column it orders by
// and the direction it goes in. Keeping them together is what lets the ORDER BY
// and the cursor be built from one description instead of two that have to
// agree.
type usersSort struct {
	column     string
	descending bool
}

func userSort(sortBy *schema.UserSort) usersSort {
	if sortBy == nil {
		return usersSort{column: "created_at", descending: true}
	}
	switch *sortBy {
	case schema.UserSortCreatedAsc:
		return usersSort{column: "created_at"}
	case schema.UserSortUsernameAsc:
		return usersSort{column: "username"}
	case schema.UserSortUsernameDesc:
		return usersSort{column: "username", descending: true}
	case schema.UserSortRankAsc:
		return usersSort{column: "rank"}
	case schema.UserSortRankDesc:
		return usersSort{column: "rank", descending: true}
	default:
		return usersSort{column: "created_at", descending: true}
	}
}

func (s usersSort) orderBy() []string {
	direction := "ASC"
	if s.descending {
		direction = "DESC"
	}
	// The id breaks ties, and paging depends on it: without a total order two
	// rows with the same timestamp could straddle a page boundary in either
	// order, and one of them would be lost.
	return []string{s.column + " " + direction, "id " + direction}
}

// cursorValue is the value a row was sorted by, as text the cursor can carry.
func (s usersSort) cursorValue(user model.User) string {
	switch s.column {
	case "username":
		return user.Username
	case "rank":
		return strconv.Itoa(user.Rank)
	default:
		return user.CreatedAt.Format(time.RFC3339Nano)
	}
}

// bind reads a cursor's value back into something the comparison can take. A
// timestamp has to come back as a time rather than as text, so that bun renders
// it in the same format the column holds.
func (s usersSort) bind(value string) (any, error) {
	switch s.column {
	case "username":
		return value, nil
	case "rank":
		return strconv.Atoi(value)
	default:
		return time.Parse(time.RFC3339Nano, value)
	}
}

// seek adds the condition "strictly after this row in this order", which is the
// whole of keyset paging.
func (s usersSort) seek(query *bun.SelectQuery, cursor, fingerprint string) (*bun.SelectQuery, error) {
	value, id, err := decodeCursor(cursor, fingerprint)
	if err != nil {
		return nil, err
	}
	bound, err := s.bind(value)
	if err != nil {
		return nil, errCursorMismatch
	}
	comparison := ">"
	if s.descending {
		comparison = "<"
	}
	condition := "(" + s.column + " " + comparison + " ? OR (" + s.column + " = ? AND id " + comparison + " ?))"
	return query.Where(condition, bound, bound, id), nil
}

// A cursor is "<fingerprint>.<sort value>.<id>", base64url encoded so it reads
// as opaque and survives a URL. The fingerprint is what enforces the schema's
// "only valid with the same filter and sortBy": a cursor names a position in one
// particular ordering of one particular set of rows, and in any other it names
// nothing.
func encodeCursor(fingerprint, sortValue string, id int64) string {
	payload := strings.Join([]string{
		fingerprint,
		base64.RawURLEncoding.EncodeToString([]byte(sortValue)),
		strconv.FormatInt(id, 10),
	}, ".")
	return base64.RawURLEncoding.EncodeToString([]byte(payload))
}

func decodeCursor(cursor, fingerprint string) (string, int64, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", 0, errCursorMismatch
	}
	parts := strings.Split(string(decoded), ".")
	if len(parts) != 3 || parts[0] != fingerprint {
		return "", 0, errCursorMismatch
	}
	sortValue, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", 0, errCursorMismatch
	}
	id, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return "", 0, errCursorMismatch
	}
	return string(sortValue), id, nil
}

// pageFingerprint identifies the query a cursor belongs to. It hashes the sort
// and every filter value, so changing any of them retires the cursors that were
// handed out under the old one rather than quietly paging through a different
// set of rows.
func pageFingerprint(sortBy *schema.UserSort, filter *schema.UserFilterInput) string {
	parts := []string{"sort"}
	if sortBy != nil {
		parts = append(parts, string(*sortBy))
	}
	if filter != nil {
		if filter.Search != nil {
			parts = append(parts, "search="+*filter.Search)
		}
		if filter.Role != nil {
			parts = append(parts, "role="+string(*filter.Role))
		}
		if filter.HasPassword != nil {
			parts = append(parts, "hasPassword="+strconv.FormatBool(*filter.HasPassword))
		}
		if filter.CreatedAfter != nil {
			parts = append(parts, "createdAfter="+filter.CreatedAfter.Format(time.RFC3339))
		}
		if filter.CreatedBefore != nil {
			parts = append(parts, "createdBefore="+filter.CreatedBefore.Format(time.RFC3339))
		}
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:4])
}

var errCursorMismatch = schema.InvalidInput(schema.InvalidInputReasonMalformed, []string{"after"},
	"this cursor does not belong to this filter and sortBy")
