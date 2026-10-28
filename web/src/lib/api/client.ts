// Typed client for /api/v1. Every call accepts an AbortSignal; errors are ApiError.
import type {
	Candidate,
	HomeData,
	ItemDetail,
	Item,
	ItemPage,
	ItemPatch,
	ItemQuery,
	JobStats,
	Library,
	LibraryFilters,
	LibraryKind,
	PlaybackInfo,
	PlaybackRequest,
	SearchResults,
	SystemInfo
} from './types';

export const API_BASE = '/api/v1';

/** API failure: `code` and `message` come from the server's error envelope when present. */
export class ApiError extends Error {
	readonly status: number;
	readonly code: string;

	constructor(status: number, code: string, message: string) {
		super(message);
		this.name = 'ApiError';
		this.status = status;
		this.code = code;
	}

	/** Endpoint not available (older server, unknown route) or resource gone. */
	get notFound() {
		return this.status === 404;
	}
}

export function isAbort(err: unknown): boolean {
	return err instanceof DOMException && err.name === 'AbortError';
}

/** Human message for any thrown value (French UI copy). */
export function errorMessage(err: unknown): string {
	if (err instanceof ApiError) return err.message;
	if (err instanceof TypeError) return 'Le serveur est injoignable.';
	return 'Une erreur inattendue est survenue.';
}

type Params = Record<string, string | number | boolean | undefined | null | (string | number)[]>;

function query(params?: Params): string {
	if (!params) return '';
	const sp = new URLSearchParams();
	for (const [k, v] of Object.entries(params)) {
		if (v === undefined || v === null || v === '' || v === false) continue;
		if (Array.isArray(v)) {
			if (v.length) sp.set(k, v.join(','));
		} else sp.set(k, String(v));
	}
	const s = sp.toString();
	return s ? `?${s}` : '';
}

interface RequestOptions {
	method?: string;
	body?: unknown;
	params?: Params;
	signal?: AbortSignal;
}

export async function request<T>(path: string, { method = 'GET', body, params, signal }: RequestOptions = {}): Promise<T> {
	const headers: Record<string, string> = { accept: 'application/json' };
	if (body !== undefined) headers['content-type'] = 'application/json';
	const res = await fetch(API_BASE + path + query(params), {
		method,
		headers,
		body: body === undefined ? undefined : JSON.stringify(body),
		signal
	});
	if (!res.ok) {
		let code = `http_${res.status}`;
		let message = res.status === 404 ? 'Ressource introuvable.' : `Erreur du serveur (${res.status}).`;
		try {
			const data = await res.json();
			if (data?.error?.message) message = String(data.error.message);
			if (data?.error?.code) code = String(data.error.code);
		} catch {
			/* not JSON */
		}
		throw new ApiError(res.status, code, message);
	}
	if (res.status === 204) return undefined as T;
	const text = await res.text();
	return (text ? JSON.parse(text) : undefined) as T;
}

/** Fills the collections the UI iterates over, so partial payloads never crash a view. */
function normalizeItem<T extends Item>(it: T): T {
	it.images ??= {};
	it.providerIds ??= {};
	it.lockedFields ??= [];
	return it;
}

function normalizeDetail(d: ItemDetail): ItemDetail {
	normalizeItem(d);
	d.genres ??= [];
	d.studios ??= [];
	d.people ??= [];
	d.ancestors ??= [];
	d.mediaFiles ??= [];
	for (const f of d.mediaFiles) f.streams ??= [];
	return d;
}

const s = (signal?: AbortSignal) => ({ signal });

export const api = {
	systemInfo: (signal?: AbortSignal) => request<SystemInfo>('/system/info', s(signal)),

	libraries: (signal?: AbortSignal) => request<Library[]>('/libraries', s(signal)),
	library: (id: number, signal?: AbortSignal) => request<Library>(`/libraries/${id}`, s(signal)),
	createLibrary: (body: { name: string; kind: LibraryKind; paths: string[] }) =>
		request<Library>('/libraries', { method: 'POST', body }),
	deleteLibrary: (id: number) => request<void>(`/libraries/${id}`, { method: 'DELETE' }),
	scanLibrary: (id: number, full = false) =>
		request<{ queued: boolean }>(`/libraries/${id}/scan`, { method: 'POST', params: { full } }),
	filters: async (id: number, signal?: AbortSignal) => {
		const f = await request<LibraryFilters>(`/libraries/${id}/filters`, s(signal));
		return { genres: f.genres ?? [], years: f.years ?? null, resolutions: f.resolutions ?? [], hasHdr: !!f.hasHdr };
	},

	items: async (q: ItemQuery, signal?: AbortSignal) => {
		const page = await request<ItemPage>('/items', { params: q as Params, signal });
		page.items = (page.items ?? []).map(normalizeItem);
		return page;
	},
	item: async (id: number, signal?: AbortSignal) => normalizeDetail(await request<ItemDetail>(`/items/${id}`, s(signal))),
	similar: async (id: number, limit = 12, signal?: AbortSignal) =>
		((await request<Item[]>(`/items/${id}/similar`, { params: { limit }, signal })) ?? []).map(normalizeItem),
	patchItem: async (id: number, body: ItemPatch) =>
		normalizeDetail(await request<ItemDetail>(`/items/${id}`, { method: 'PATCH', body })),
	refreshItem: (id: number, research = false) =>
		request<{ queued: boolean }>(`/items/${id}/refresh`, { method: 'POST', params: { research } }),
	identifySearch: async (id: number, body: { title?: string; year?: number }, signal?: AbortSignal) =>
		(await request<Candidate[]>(`/items/${id}/identify/search`, { method: 'POST', body, signal })) ?? [],
	identify: (id: number, providerIds: Record<string, string>) =>
		request<void>(`/items/${id}/identify`, { method: 'POST', body: { providerIds } }),

	home: async (signal?: AbortSignal) => {
		const h = await request<HomeData>('/home', s(signal));
		return {
			hero: (h.hero ?? []).map(normalizeItem),
			continueWatching: (h.continueWatching ?? []).map(normalizeItem),
			recentlyAdded: (h.recentlyAdded ?? []).map((r) => ({ ...r, items: (r.items ?? []).map(normalizeItem) })),
			categories: h.categories ?? [],
			recommended: (h.recommended ?? []).map(normalizeItem)
		} satisfies HomeData;
	},

	search: async (q: string, limit = 8, signal?: AbortSignal) => {
		const r = await request<SearchResults>('/search', { params: { q, limit }, signal });
		const res = r.results ?? ({} as SearchResults['results']);
		return {
			query: r.query ?? q,
			results: {
				movie: (res.movie ?? []).map(normalizeItem),
				series: (res.series ?? []).map(normalizeItem),
				episode: (res.episode ?? []).map(normalizeItem),
				artist: (res.artist ?? []).map(normalizeItem),
				album: (res.album ?? []).map(normalizeItem),
				track: (res.track ?? []).map(normalizeItem),
				person: res.person ?? []
			}
		} satisfies SearchResults;
	},

	jobs: (signal?: AbortSignal) => request<JobStats>('/jobs', s(signal)),

	// ——— Lecture (M5) ———
	startPlayback: async (itemId: number, body: PlaybackRequest, signal?: AbortSignal) => {
		const p = await request<PlaybackInfo>(`/playback/${itemId}`, { method: 'POST', body, signal });
		p.reasons ??= [];
		p.audioTracks ??= [];
		p.subtitleTracks ??= [];
		p.selectedSubtitle ??= -1;
		p.startMs ??= 0;
		return p;
	},
	pingPlayback: (sessionId: string, body: { positionMs: number; paused: boolean }) =>
		request<void>(`/playback/sessions/${encodeURIComponent(sessionId)}/ping`, { method: 'POST', body }),
	/**
	 * Stops a session. `keepalive` lets the request outlive the page (pagehide / unload):
	 * sendBeacon cannot send DELETE.
	 */
	stopPlayback: (sessionId: string, keepalive = false) =>
		fetch(`${API_BASE}/playback/sessions/${encodeURIComponent(sessionId)}`, { method: 'DELETE', keepalive }).then(
			() => undefined,
			() => undefined
		)
};
