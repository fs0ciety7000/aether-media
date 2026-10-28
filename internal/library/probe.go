package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/jobs"
	"github.com/fs0ciety7000/aether-media/internal/media/probe"
	"github.com/fs0ciety7000/aether-media/internal/sources"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
)

type probePayload struct {
	MediaFileID int64 `json:"mediaFileId"`
}

// ProbeHandler returns the jobs.Handler for JobProbe: it runs ffprobe on a
// media file and stores container, streams and chapters. Audio tags refine
// track titles and numbers.
// agentResumeDelay lets the breaker's probe of an agent conclude before the
// postponed analyses come back.
const agentResumeDelay = 5 * time.Second

func (s *Scanner) ProbeHandler(p *probe.Prober) jobs.Handler {
	return func(ctx context.Context, j jobs.Job) error {
		var pl probePayload
		if err := json.Unmarshal(j.Payload, &pl); err != nil {
			return fmt.Errorf("%w: %w", jobs.ErrPermanent, err)
		}
		mf, err := s.store.GetMediaFile(ctx, pl.MediaFileID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil // file removed since the job was queued
		}
		if err != nil {
			return err
		}

		info, err := s.Sources.Probe(ctx, p, mf.Path)
		if errors.Is(err, probe.ErrNotMedia) {
			s.log.Warn("fichier illisible", "path", mf.Path, "err", err)
			return s.store.SetProbeError(ctx, db.SetProbeErrorParams{ProbeError: nullStr(err.Error()), ID: mf.ID})
		}
		if de, ok := errors.AsType[*sources.AgentDownError](err); ok {
			// The agent is behind an open circuit breaker: wait for its
			// next probe instead of each job retrying on its own.
			return jobs.Postpone(err, de.RetryIn()+agentResumeDelay)
		}
		if err != nil {
			if j.Attempts >= 3 {
				_ = s.store.SetProbeError(context.WithoutCancel(ctx), db.SetProbeErrorParams{ProbeError: nullStr(err.Error()), ID: mf.ID})
			}
			return err
		}
		var subs []sidecar
		if mf.ItemKind != "track" {
			// Listed outside the transaction: remote directories may be slow.
			if entries, err := s.Sources.ReadDir(ctx, filepath.Dir(mf.Path)); err == nil {
				subs = findSidecars(mf.Path, entries)
			}
		}
		return s.store.Tx(ctx, func(q *db.Queries) error { return saveProbe(ctx, q, mf, info, subs) })
	}
}

func saveProbe(ctx context.Context, q *db.Queries, mf db.GetMediaFileRow, info *probe.Info, subs []sidecar) error {
	err := q.SetProbeResult(ctx, db.SetProbeResultParams{
		Container:  nullStr(info.Container),
		DurationMs: sql.NullInt64{Int64: info.DurationMs, Valid: info.DurationMs > 0},
		Bitrate:    sql.NullInt64{Int64: info.Bitrate, Valid: info.Bitrate > 0},
		ID:         mf.ID,
	})
	if err != nil {
		return err
	}
	if err := q.DeleteMediaStreams(ctx, mf.ID); err != nil {
		return err
	}
	for _, st := range info.Streams {
		if st.IsAttachedPic {
			continue
		}
		err := q.InsertMediaStream(ctx, db.InsertMediaStreamParams{
			MediaFileID: mf.ID, Idx: int64(st.Index), Type: st.Type, Codec: st.Codec, Profile: st.Profile,
			Level: int64(st.Level), Width: int64(st.Width), Height: int64(st.Height), FrameRate: st.FrameRate,
			BitDepth: int64(st.BitDepth), Hdr: st.HDR, Channels: int64(st.Channels), ChannelLayout: st.ChannelLayout,
			SampleRate: int64(st.SampleRate), Language: st.Language, Title: st.Title,
			IsDefault: b2i(st.Default), IsForced: b2i(st.Forced), IsHearingImpaired: b2i(st.HearingImpaired),
			IsImageSub: b2i(st.IsImageSub), Bitrate: st.Bitrate,
		})
		if err != nil {
			return err
		}
	}
	if err := q.DeleteChapters(ctx, mf.ID); err != nil {
		return err
	}
	for i, c := range info.Chapters {
		err := q.InsertChapter(ctx, db.InsertChapterParams{
			MediaFileID: mf.ID, Idx: int64(i), StartMs: c.StartMs, EndMs: c.EndMs, Title: c.Title,
		})
		if err != nil {
			return err
		}
	}
	if mf.ItemKind != "track" {
		if err := saveSidecars(ctx, q, mf.ID, subs); err != nil {
			return err
		}
	}
	if info.DurationMs > 0 && mf.Part <= 1 {
		if err := q.SetItemRuntime(ctx, db.SetItemRuntimeParams{RuntimeMs: sql.NullInt64{Int64: info.DurationMs, Valid: true}, ID: mf.ItemID}); err != nil {
			return err
		}
	}
	if mf.ItemKind == "track" {
		return applyTrackTags(ctx, q, mf.ItemID, info.Tags)
	}
	return nil
}

// applyTrackTags refines a track from its embedded tags (title, track and
// disc numbers, year), keeping path-derived values where tags are missing.
func applyTrackTags(ctx context.Context, q *db.Queries, itemID int64, tags map[string]string) error {
	title := strings.TrimSpace(tags["title"])
	if title == "" {
		return nil
	}
	it, err := q.GetItem(ctx, itemID)
	if err != nil {
		return err
	}
	num, disc := tagNumber(tags["track"]), tagNumber(tags["disc"])
	if !num.Valid {
		num = it.IndexNumber
	}
	if !disc.Valid {
		disc = it.ParentIndexNumber
	}
	n := node{kind: "track", title: title}
	if num.Valid {
		n.index = intp(int(num.Int64))
	}
	if disc.Valid {
		n.parentIndex = intp(int(disc.Int64))
	}
	return q.UpdateTrackFromTags(ctx, db.UpdateTrackFromTagsParams{
		Title:             title,
		SortTitle:         sortKey(n),
		IndexNumber:       num,
		ParentIndexNumber: disc,
		Year:              tagNumber(firstN(tags["date"], 4)),
		ID:                itemID,
	})
}

// tagNumber parses "3", "03/12" or "1-2" style tag values.
func tagNumber(v string) sql.NullInt64 {
	v = strings.TrimSpace(v)
	if i := strings.IndexAny(v, "/-"); i > 0 {
		v = v[:i]
	}
	n, err := strconv.Atoi(v)
	return sql.NullInt64{Int64: int64(n), Valid: err == nil && n > 0}
}

func firstN(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
