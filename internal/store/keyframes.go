package store

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"
)

// maxKeyframes bounds a stored list (a 4-hour film with a keyframe every
// 0.5 s has under 30 000).
const maxKeyframes = 500_000

// LoadKeyframes returns the stored keyframe times (seconds) of a media file
// when they were computed for this version of it.
func (s *Store) LoadKeyframes(ctx context.Context, mediaFileID int64, version string) ([]float64, bool) {
	var stored string
	var count int64
	var data []byte
	err := s.DB.QueryRowContext(ctx,
		`SELECT version, count, data FROM media_keyframes WHERE media_file_id = ?`, mediaFileID).Scan(&stored, &count, &data)
	if err != nil || stored != version {
		return nil, false
	}
	kf, err := decodeKeyframes(data, count)
	if err != nil {
		return nil, false
	}
	return kf, true
}

// SaveKeyframes stores the keyframe times (seconds) of a media file version.
// Errors are not fatal (the list is only a cache): they are returned for
// the caller to log.
func (s *Store) SaveKeyframes(ctx context.Context, mediaFileID int64, version string, kf []float64) error {
	if len(kf) == 0 || len(kf) > maxKeyframes {
		return fmt.Errorf("store: %d keyframes", len(kf))
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO media_keyframes (media_file_id, version, count, data, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (media_file_id) DO UPDATE SET version = excluded.version, count = excluded.count,
			data = excluded.data, created_at = excluded.created_at`,
		mediaFileID, version, len(kf), encodeKeyframes(kf), time.Now().Unix())
	return err
}

// encodeKeyframes: times in microseconds, zig-zag varint deltas.
func encodeKeyframes(kf []float64) []byte {
	out := make([]byte, 0, len(kf)*3)
	var prev int64
	for _, t := range kf {
		us := int64(math.Round(t * 1e6))
		out = binary.AppendVarint(out, us-prev)
		prev = us
	}
	return out
}

func decodeKeyframes(data []byte, count int64) ([]float64, error) {
	if count <= 0 || count > maxKeyframes {
		return nil, errors.New("store: invalid keyframe count")
	}
	kf := make([]float64, 0, count)
	var prev int64
	for len(data) > 0 {
		d, n := binary.Varint(data)
		if n <= 0 {
			return nil, errors.New("store: corrupt keyframe list")
		}
		data = data[n:]
		prev += d
		kf = append(kf, float64(prev)/1e6)
	}
	if int64(len(kf)) != count {
		return nil, sql.ErrNoRows
	}
	return kf, nil
}
