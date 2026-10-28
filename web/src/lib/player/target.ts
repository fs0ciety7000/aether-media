// What "Lecture" plays for each kind, and the player URL.
import { api } from '$lib/api/client';
import type { Item, Kind } from '$lib/api/types';
import { latestResume, resumeFor } from './resume';

/** Kinds the player opens directly. */
export const PLAYABLE: Kind[] = ['movie', 'episode', 'track'];
/** Kinds with a "Lecture" button (resolved to a playable child). */
export const HAS_PLAY: Kind[] = ['movie', 'episode', 'track', 'series', 'season', 'album'];

export function playerHref(id: number, opts: { t?: number; audio?: number; sub?: number } = {}): string {
	const sp = new URLSearchParams();
	if (opts.t) sp.set('t', String(Math.round(opts.t)));
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

/** Regular seasons first; specials (season 0) only when nothing else exists. */
function firstSeason(seasons: Item[]): Item | undefined {
	return seasons.find((s) => (s.indexNumber ?? 1) > 0) ?? seasons[0];
}

/**
 * Item to play for `item`: itself (movie, episode, track), the in-progress episode of a season
 * (else its first), the first episode of a series' first season, the first track of an album.
 * `known` lets callers pass already-loaded children (episodes of a season, tracks…).
 */
export async function resolvePlayTarget(item: Pick<Item, 'id' | 'kind'>, known?: Item[], signal?: AbortSignal): Promise<number | null> {
	if (PLAYABLE.includes(item.kind)) return item.id;
	if (item.kind === 'season') {
		const eps = known?.length ? known : await children(item.id, 'episode', signal);
		return latestResume(eps.map((e) => e.id))?.id ?? eps[0]?.id ?? null;
	}
	if (item.kind === 'series') {
		const season = firstSeason(await children(item.id, 'season', signal));
		if (!season) return null;
		const eps = await children(season.id, 'episode', signal);
		return latestResume(eps.map((e) => e.id))?.id ?? eps[0]?.id ?? null;
	}
	if (item.kind === 'album') {
		const tracks = known?.length ? [...known].sort(byIndex) : await children(item.id, 'track', signal);
		return tracks[0]?.id ?? null;
	}
	return null;
}

/** "Reprendre" when a local resume point exists for the item or one of its known children. */
export function hasResume(item: Pick<Item, 'id' | 'kind'>, known: Item[] = []): boolean {
	if (item.kind === 'movie' || item.kind === 'episode') return !!resumeFor(item.id);
	if (item.kind === 'season' || item.kind === 'series') return !!latestResume(known.map((e) => e.id));
	return false;
}

/** Next episode in the same season / next track in the same album (simple version). */
export async function nextSibling(item: Pick<Item, 'id' | 'kind' | 'parentId'>, signal?: AbortSignal): Promise<Item | null> {
	if (!item.parentId || (item.kind !== 'episode' && item.kind !== 'track')) return null;
	const list = await children(item.parentId, item.kind, signal);
	const i = list.findIndex((x) => x.id === item.id);
	return i >= 0 ? (list[i + 1] ?? null) : null;
}
