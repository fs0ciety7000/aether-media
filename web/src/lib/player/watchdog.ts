// Player watchdogs: a playback must end either playing or with a clear error, never with an
// endless spinner (slow remote source, stuck transcode, native HLS that silently retries).
// Pure (no Svelte, no DOM): unit-tested by `npm run test:player`.

/** POST /playback: the server bounds its own preparation (keyframes ≤ 40 s), so this is generous. */
export const START_TIMEOUT_MS = 90_000;
/** Session attached but no image yet (first HLS segment or first bytes of the file). */
export const FIRST_FRAME_TIMEOUT_MS = 75_000;
/** Playing, then stuck buffering without progress. */
export const STALL_TIMEOUT_MS = 90_000;

export interface WatchState {
	phase: 'loading' | 'prompt' | 'starting' | 'playing' | 'error';
	firstFrame: boolean;
	/** The element is waiting for data (or the engine is recovering / restarting). */
	waiting: boolean;
	paused: boolean;
}

export type Watch = { ms: number; message: string } | null;

export const FIRST_FRAME_MESSAGE =
	'La vidéo ne démarre pas : le serveur n’a fourni aucune image à temps. La source est peut-être trop lente (agent distant) ou le transcodage trop lourd pour ce serveur.';
export const STALL_MESSAGE =
	'La lecture est bloquée : le serveur ne fournit plus la vidéo assez vite. La liaison avec la source est peut-être trop lente pour ce fichier.';

/** Which watchdog applies to the player state (null: none). */
export function watchFor(s: WatchState): Watch {
	if (s.phase !== 'playing') return null;
	if (!s.firstFrame) return { ms: FIRST_FRAME_TIMEOUT_MS, message: FIRST_FRAME_MESSAGE };
	if (s.waiting && !s.paused) return { ms: STALL_TIMEOUT_MS, message: STALL_MESSAGE };
	return null;
}

/** True for the error a fetch raises when AbortSignal.timeout() fires. */
export function isTimeout(err: unknown): boolean {
	return typeof err === 'object' && err !== null && (err as { name?: string }).name === 'TimeoutError';
}
