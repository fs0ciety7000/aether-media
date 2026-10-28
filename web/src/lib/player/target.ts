// What "Lecture" plays for each kind, and the player URL.
import { api } from '$lib/api/client';
import type { Item, Kind } from '$lib/api/types';
import { resumeMs } from './resume';

/** Kinds the player opens directly. */
export const PLAYABLE: Kind[] = ['movie', 'episode', 'track'];
/** Kinds with a "Lecture" button (resolved to a playable child). */
export const HAS_PLAY: Kind[] = ['movie', 'episode', 'track', 'series', 'season', 'album'];

export function playerHref(id: number, opts: { t?: number; audio?: number; sub?: number } = {}): string {
	const sp = new URLSearchParams();
	// t=0 is meaningful: "Depuis le début" skips the resume prompt.
	if (opts.t !== undefined) sp.set('t', String(Math.round(opts.t)));
	if (opts.audio !== undefined) sp.set('audio', String(opts.audio));
	if (opts.sub !== undefined) sp.set('sub', String(opts.sub));
	const q = sp.toString();
	return `/lecture/${id}${q ? `?${q}` : ''}`;
}

export const byIndex = (a: Item, b: Item) =>
	(a.parentIndexNumber ?? 0) - (b.parentIndexNumber ?? 0) || (a.indexNumber ?? 1e9) - (b.indexNumber ?? 1e9) || a.title.localeCompare(b.title, 'fr');

async function children(parent: number, kind: Kind, signal?: AbortSignal): Promise<Item[]> {
	const page = await api.items({ parent, kind, limit: 200 }, signal);
	return [...page.items].sort(byIndex);
}

/** In a season (sorted): the in-progress episode, else the first unwatched one. */
export function nextInSeason(eps: Item[]): Item | undefined {
	return eps.find((e) => resumeMs(e) > 0 && !e.userData?.played) ?? eps.find((e) => !e.userData?.played);
}

/** Seasons in watching order: regular seasons, then specials (season 0). */
function watchOrder(seasons: Item[]): Item[] {
	return [...seasons.filter((s) => (s.indexNumber ?? 1) > 0), ...seasons.filter((s) => (s.indexNumber ?? 1) <= 0)];
}

/**
 * Next-up episode of a series: the first unwatched (or in-progress) episode in order,
 * skipping fully watched seasons. Everything watched: the very first episode.
 */
export async function seriesNextUp(seriesId: number, signal?: AbortSignal, knownSeasons?: Item[]): Promise<Item | null> {
	const seasons = watchOrder(knownSeasons?.length ? [...knownSeasons].sort(byIndex) : await children(seriesId, 'season', signal));
	let first: Item | undefined;
	for (const season of seasons) {
		// unplayedCount === 0: nothing left there (missing on older servers: look inside).
		if (season.unplayedCount === 0 && first) continue;
		const eps = await children(season.id, 'episode', signal);
		first ??= eps[0];
		if (season.unplayedCount === 0) continue;
		const next = nextInSeason(eps);
		if (next) return next;
	}
	return first ?? null;
}

/**
 * Item to play for `item`: itself (movie, episode, track), the in-progress or first unwatched
 * episode of a season (else its first), the next-up episode of a series, the first track of an album.
 * `known` lets callers pass already-loaded children (episodes of a season, tracks…).
 */
export async function resolvePlayTarget(item: Pick<Item, 'id' | 'kind'>, known?: Item[], signal?: AbortSignal): Promise<number | null> {
	if (PLAYABLE.includes(item.kind)) return item.id;
	if (item.kind === 'season') {
		const eps = known?.length ? [...known].sort(byIndex) : await children(item.id, 'episode', signal);
		return (nextInSeason(eps) ?? eps[0])?.id ?? null;
	}
	if (item.kind === 'series') {
		return (await seriesNextUp(item.id, signal))?.id ?? null;
	}
	if (item.kind === 'album') {
		const tracks = known?.length ? [...known].sort(byIndex) : await children(item.id, 'track', signal);
		return tracks[0]?.id ?? null;
	}
	return null;
}


/** Next episode in the same season / next track in the same album (simple version). */
export async function nextSibling(item: Pick<Item, 'id' | 'kind' | 'parentId'>, signal?: AbortSignal): Promise<Item | null> {
	if (!item.parentId || (item.kind !== 'episode' && item.kind !== 'track')) return null;
	const list = await children(item.parentId, item.kind, signal);
	const i = list.findIndex((x) => x.id === item.id);
	return i >= 0 ? (list[i + 1] ?? null) : null;
}
