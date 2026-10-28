// Watch progress from the server (M6): Item.userData on movies, episodes and tracks,
// Item.unplayedCount on series and seasons. The player's pings store the position;
// closing the session marks the title as played past ~90 %.
import type { Item } from '$lib/api/types';

/** Below this, a stored position is not worth a "Reprendre" (accidental clicks). */
export const RESUME_MIN_MS = 5_000;

type WithData = Pick<Item, 'userData' | 'runtimeMs'>;

export function isPlayed(item: Pick<Item, 'userData' | 'unplayedCount' | 'kind' | 'childCount'> | null | undefined): boolean {
	if (!item) return false;
	if (item.kind === 'series' || item.kind === 'season') return item.unplayedCount === 0 && (item.childCount ?? 1) > 0;
	return !!item.userData?.played;
}

/** Resume position in ms (0 when there is nothing to resume). */
export function resumeMs(item: WithData | null | undefined): number {
	const p = item?.userData?.positionMs ?? 0;
	if (p < RESUME_MIN_MS) return 0;
	if (item?.runtimeMs && p >= item.runtimeMs) return 0;
	return p;
}

/** 0–1 share watched for in-progress titles, undefined otherwise. */
export function progressOf(item: WithData | null | undefined): number | undefined {
	const p = resumeMs(item);
	if (!p || !item?.runtimeMs) return undefined;
	return Math.min(1, p / item.runtimeMs);
}

export function isFavorite(item: Pick<Item, 'userData'> | null | undefined): boolean {
	return !!item?.userData?.favorite;
}
