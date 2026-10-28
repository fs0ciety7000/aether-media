package store

import (
	"context"
	"database/sql"

	"github.com/fs0ciety7000/aether-media/internal/auth"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
)

// UpdateRatingAge recomputes items.rating_age from the item's official
// rating (parental controls filter on it).
func UpdateRatingAge(ctx context.Context, q *db.Queries, itemID int64) error {
	it, err := q.GetItem(ctx, itemID)
	if err != nil {
		return err
	}
	age := sql.NullInt64{}
	if a, ok := auth.RatingAge(it.OfficialRating.String); ok {
		age = sql.NullInt64{Int64: int64(a), Valid: true}
	}
	return q.SetItemRatingAge(ctx, db.SetItemRatingAgeParams{RatingAge: age, ID: itemID})
}

// BackfillRatingAges fills rating_age for items rated before it existed.
func (s *Store) BackfillRatingAges(ctx context.Context) error {
	rows, err := s.ListItemsForRatingAge(ctx)
	if err != nil {
		return err
	}
	return s.Tx(ctx, func(q *db.Queries) error {
		for _, r := range rows {
			age := sql.NullInt64{}
			if a, ok := auth.RatingAge(r.OfficialRating.String); ok {
				age = sql.NullInt64{Int64: int64(a), Valid: true}
			}
			if err := q.SetItemRatingAge(ctx, db.SetItemRatingAgeParams{RatingAge: age, ID: r.ID}); err != nil {
				return err
			}
		}
		return nil
	})
}
