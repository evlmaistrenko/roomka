package accounts

import (
	"context"
	"fmt"

	"control/internal/model"
	"control/internal/schema"
)

// Preferences are the one place a client writes whatever it likes. The server
// stores the bytes and hands them back: it does not parse them, and it does not
// know what any key means. What it does know is how many there may be and how
// long each may get, because a store the client defines is a store the client
// can fill.

// loadPreferences reads a user's preferences, ordered by key so that a client
// diffing two responses sees a change only when something changed.
func (s *Service) loadPreferences(ctx context.Context, userID int64) ([]*schema.Preference, error) {
	var stored []model.UserPreference
	if err := s.database.NewSelect().
		Model(&stored).
		Where("user_id = ?", userID).
		Order("name").
		Scan(ctx); err != nil {
		return nil, err
	}

	preferences := make([]*schema.Preference, 0, len(stored))
	for _, one := range stored {
		preferences = append(preferences, &schema.Preference{Key: one.Name, Value: one.Value})
	}
	return preferences, nil
}

// setPreference writes one key, or removes it when the value is null, and
// answers with the whole set: a client that keeps preferences in one object
// needs no merge to apply the result.
func (s *Service) SetPreference(ctx context.Context, key string, input schema.SetPreferenceInput) ([]*schema.Preference, error) {
	principal, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	if input.Value == nil {
		if _, err := s.database.NewDelete().
			Model((*model.UserPreference)(nil)).
			Where("user_id = ?", principal.UserID).
			Where("name = ?", key).
			Exec(ctx); err != nil {
			return nil, err
		}
	} else {
		if err := s.checkPreferenceLimit(ctx, principal.UserID, key); err != nil {
			return nil, err
		}
		if _, err := s.database.NewInsert().
			Model(&model.UserPreference{UserID: principal.UserID, Name: key, Value: *input.Value}).
			On("CONFLICT (user_id, name) DO UPDATE").
			Set("value = EXCLUDED.value").
			Exec(ctx); err != nil {
			return nil, err
		}
	}

	s.publishViewer(principal.UserID)
	return s.loadPreferences(ctx, principal.UserID)
}

// checkPreferenceLimit refuses a new key once the cap is full. Overwriting an
// existing key is always allowed, so a client at the cap can still work — it
// just cannot grow.
func (s *Service) checkPreferenceLimit(ctx context.Context, userID int64, key string) error {
	existing, err := s.database.NewSelect().
		Model((*model.UserPreference)(nil)).
		Where("user_id = ?", userID).
		Where("name = ?", key).
		Exists(ctx)
	if err != nil {
		return err
	}
	if existing {
		return nil
	}

	stored, err := s.database.NewSelect().
		Model((*model.UserPreference)(nil)).
		Where("user_id = ?", userID).
		Count(ctx)
	if err != nil {
		return err
	}
	if stored >= maxPreferencesPerUser {
		return schema.Coded(schema.ErrorCodeLimitReached,
			fmt.Sprintf("at most %d preferences may be stored", maxPreferencesPerUser))
	}
	return nil
}
