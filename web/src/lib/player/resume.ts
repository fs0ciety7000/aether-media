// Local resume positions (server-side progress arrives in M6).
// Key: `aether.resume.<itemId>` → JSON { p: positionMs, d: durationMs, t: updatedAt }.

const PREFIX = 'aether.resume.';
/** Below this, starting over is the natural choice. */
export const RESUME_MIN_MS = 30_000;
/** Past this share of the runtime the title counts as watched. */
export const WATCHED_RATIO = 0.95;

export interface ResumePoint {
	positionMs: number;
	durationMs: number;
	updatedAt: number;
}

export function readResume(itemId: number): ResumePoint | null {
	try {
		const raw = localStorage.getItem(PREFIX + itemId);
		if (!raw) return null;
		const v = JSON.parse(raw);
		if (typeof v === 'number') return { positionMs: v, durationMs: 0, updatedAt: 0 };
		if (typeof v?.p !== 'number') return null;
		return { positionMs: v.p, durationMs: Number(v.d) || 0, updatedAt: Number(v.t) || 0 };
	} catch {
		return null;
	}
}

/** A position worth offering: past 30 s and before 95 % (unknown duration: past 30 s). */
export function isResumable(r: ResumePoint | null | undefined): r is ResumePoint {
	if (!r || r.positionMs < RESUME_MIN_MS) return false;
	return !r.durationMs || r.positionMs < r.durationMs * WATCHED_RATIO;
}

export function resumeFor(itemId: number): ResumePoint | null {
	const r = readResume(itemId);
	return isResumable(r) ? r : null;
}

/** Saves the position, or clears it once the title is (almost) finished. */
export function saveResume(itemId: number, positionMs: number, durationMs: number) {
	try {
		if (durationMs > 0 && positionMs >= durationMs * WATCHED_RATIO) localStorage.removeItem(PREFIX + itemId);
		else if (positionMs >= RESUME_MIN_MS) localStorage.setItem(PREFIX + itemId, JSON.stringify({ p: Math.round(positionMs), d: Math.round(durationMs), t: Date.now() }));
	} catch {
		/* storage unavailable */
	}
}

export function clearResume(itemId: number) {
	try {
		localStorage.removeItem(PREFIX + itemId);
	} catch {
		/* ignore */
	}
}

/** Most recently updated resumable item among `ids`. */
export function latestResume(ids: number[]): { id: number; point: ResumePoint } | null {
	let best: { id: number; point: ResumePoint } | null = null;
	for (const id of ids) {
		const point = resumeFor(id);
		if (point && (!best || point.updatedAt > best.point.updatedAt)) best = { id, point };
	}
	return best;
}
