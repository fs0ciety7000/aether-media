// Typed client for /api/v1. Every call accepts an AbortSignal; errors are ApiError.
import type {
	Agent,
	AgentShare,
	AuthResult,
	DiscoveredAgent,
	FolderListing,
	Candidate,
	Collection,
	DeviceSession,
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
	Source,
	SourceInput,
	SourcePatch,
	SourceTestResult,
	SystemInfo,
	User,
	UserData,
	UserDataPatch,
	UserInput,
	MePatch,
	SyncApp,
	SyncApps,
	SyncConnection,
	SyncEvent,
	SyncFeature,
	SyncPollResult,
	SyncService,
	DeviceCode,
	ActiveSession,
	AdminTask,
	JobQueue,
	LogPage,
	ServerSettings,
	SettingsPatch,
	SystemStatus,
	TmdbCheck
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

/** UI copy for known server error codes (the server's own messages are terse, lower-case). */
const CODE_MESSAGES: Record<string, string> = {
	invalid_credentials: 'Nom ou mot de passe incorrect.',
	rate_limited: 'Trop de tentatives. Réessayez dans quelques minutes.',
	unauthenticated: 'Votre session a expiré. Reconnectez-vous.',
	forbidden: 'Action réservée aux administrateurs.',
	weak_password: 'Le mot de passe doit faire au moins 8 caractères.',
	wrong_password: 'Le mot de passe actuel est incorrect.',
	name_taken: 'Ce nom est déjà utilisé.',
	last_admin: 'Impossible : c’est le dernier administrateur.',
	already_setup: 'Aether est déjà configuré.',
	bad_origin: 'Requête refusée (origine inattendue).',
	// M6b — synchronisation
	sync_unavailable: 'Ce service n’est pas configuré sur ce serveur.',
	not_connected: 'Aucun compte n’est relié à ce service.',
	not_pending: 'Aucun appairage en cours : demandez un nouveau code.',
	reauth_required: 'Le service a refusé l’accès : reconnectez votre compte.',
	managed_by_config: 'Ce réglage est défini par la configuration du serveur.',
	// M7 — administration
	not_retryable: 'Seule une tâche en échec peut être relancée (ou la même tâche est déjà en file).',
	not_cancellable: 'Cette tâche ne peut plus être annulée.',
	invalid_task: 'Tâche inconnue.',
	invalid_kind: 'Seuls les films et les séries peuvent aller dans « Ma liste ».'
};

/** Human message for any thrown value (French UI copy). */
export function errorMessage(err: unknown): string {
	if (err instanceof ApiError) return CODE_MESSAGES[err.code] ?? sentence(err.message);
	if (err instanceof TypeError) return 'Le serveur est injoignable.';
	return 'Une erreur inattendue est survenue.';
}

function sentence(m: string): string {
	const t = m.trim();
	if (!t) return t;
	return t.charAt(0).toUpperCase() + t.slice(1) + (/[.!?…]$/.test(t) ? '' : '.');
}

// ——— Global auth handling (registered by the root layout) ———

export interface AuthHandlers {
	/** 401 on a request that expects a session: send the user to the login page. */
	onUnauthenticated?: () => void;
	/** 403: the current user may not do this. */
	onForbidden?: (err: ApiError) => void;
}

let handlers: AuthHandlers = {};

export function setAuthHandlers(h: AuthHandlers) {
	handlers = h;
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
	/**
	 * false: a 401/403 is the caller's business (login form, session probe) —
	 * no redirect, no toast.
	 */
	auth?: boolean;
	/** Survives the page (pagehide). */
	keepalive?: boolean;
	/** Keep integer ids beyond 2^53 exact (as strings): JSON.parse would round them. */
	bigIds?: boolean;
}

export async function request<T>(path: string, { method = 'GET', body, params, signal, auth = true, keepalive, bigIds }: RequestOptions = {}): Promise<T> {
	const headers: Record<string, string> = { accept: 'application/json' };
	if (body !== undefined) headers['content-type'] = 'application/json';
	const res = await fetch(API_BASE + path + query(params), {
		method,
		headers,
		body: body === undefined ? undefined : JSON.stringify(body),
		signal,
		keepalive,
		credentials: 'same-origin'
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
		const err = new ApiError(res.status, code, message);
		if (auth && res.status === 401) handlers.onUnauthenticated?.();
		else if (auth && res.status === 403) handlers.onForbidden?.(err);
		throw err;
	}
	if (res.status === 204) return undefined as T;
	let text = await res.text();
	if (bigIds) text = text.replace(/"id"\s*:\s*(\d{16,})/g, '"id":"$1"');
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

/** Older servers have no `sources`: the local folders in `paths` stand in for them. */
export function normalizeLibrary(lib: Library): Library {
	lib.paths ??= [];
	lib.counts ??= {};
	if (!Array.isArray(lib.sources)) {
		lib.sources = lib.paths.map((p, i) => ({ id: -(i + 1), kind: 'local', name: p, root: p, status: 'unknown', derived: true }));
	} else {
		for (const src of lib.sources) normalizeSource(src);
	}
	return lib;
}

export function normalizeSource(src: Source): Source {
	src.status ??= 'unknown';
	src.root ??= '';
	src.name ||= src.root || src.agent?.hostname || src.sftp?.host || 'Source';
	return src;
}

export type CreateLibraryBody = { name: string; kind: LibraryKind } & ({ sources: SourceInput[] } | { paths: string[] });

const s = (signal?: AbortSignal) => ({ signal });

export const api = {
	systemInfo: (signal?: AbortSignal) => request<SystemInfo>('/system/info', s(signal)),

	libraries: async (signal?: AbortSignal) => ((await request<Library[]>('/libraries', s(signal))) ?? []).map(normalizeLibrary),
	library: async (id: number, signal?: AbortSignal) => normalizeLibrary(await request<Library>(`/libraries/${id}`, s(signal))),
	createLibrary: async (body: CreateLibraryBody) => normalizeLibrary(await request<Library>('/libraries', { method: 'POST', body })),
	deleteLibrary: (id: number) => request<void>(`/libraries/${id}`, { method: 'DELETE' }),
	scanLibrary: (id: number, full = false) =>
		request<{ queued: boolean }>(`/libraries/${id}/scan`, { method: 'POST', params: { full } }),
	filters: async (id: number, signal?: AbortSignal) => {
		const f = await request<LibraryFilters>(`/libraries/${id}/filters`, s(signal));
		return { genres: f.genres ?? [], years: f.years ?? null, resolutions: f.resolutions ?? [], hasHdr: !!f.hasHdr };
	},

	// ——— Sources (M5b) ———
	testSource: async (body: SourceInput, signal?: AbortSignal) =>
		(await request<SourceTestResult>('/sources/test', { method: 'POST', body, signal })) ?? { ok: false },
	addSource: async (libraryId: number, body: SourceInput) =>
		normalizeSource(await request<Source>(`/libraries/${libraryId}/sources`, { method: 'POST', body })),
	patchSource: async (libraryId: number, sourceId: number, body: SourcePatch) =>
		normalizeSource(await request<Source>(`/libraries/${libraryId}/sources/${sourceId}`, { method: 'PATCH', body })),
	deleteSource: (libraryId: number, sourceId: number) =>
		request<void>(`/libraries/${libraryId}/sources/${sourceId}`, { method: 'DELETE' }),

	// ——— Agents appairés, sélecteurs de dossiers (M5c) ———
	agents: async (signal?: AbortSignal) => ((await request<Agent[]>('/agents', s(signal))) ?? []).map(normalizeAgent),
	/** LAN broadcast: answers after ~2 s. */
	discoverAgents: async (signal?: AbortSignal) => (await request<DiscoveredAgent[]>('/agents/discover', { method: 'POST', signal })) ?? [],
	/** `agentId` re-pairs that agent (new certificate, HTTP → HTTPS). */
	pairAgent: async (body: { url: string; code: string; agentId?: number }, signal?: AbortSignal) =>
		normalizeAgent(await request<Agent>('/agents/pair', { method: 'POST', body, signal })),
	deleteAgent: (id: number) => request<void>(`/agents/${id}`, { method: 'DELETE' }),
	agentBrowse: async (id: number, path = '', signal?: AbortSignal) =>
		normalizeListing(await request<FolderListing>(`/agents/${id}/browse`, { params: { path }, signal }), path),
	agentShares: async (id: number, signal?: AbortSignal) => (await request<AgentShare[]>(`/agents/${id}/shares`, s(signal))) ?? [],
	addAgentShare: (id: number, body: { path: string; name?: string }) =>
		request<AgentShare>(`/agents/${id}/shares`, { method: 'POST', body }),
	/** Folders of the server itself (picker for local sources). */
	fsBrowse: async (path = '', signal?: AbortSignal) => normalizeListing(await request<FolderListing>('/fs/browse', { params: { path }, signal }), path),

	items: async (q: ItemQuery, signal?: AbortSignal) => {
		// `played=false` is a real filter: query() drops false values, so send it as text.
		const params = { ...q, played: q.played === undefined ? undefined : String(q.played) } as Params;
		const page = await request<ItemPage>('/items', { params, signal });
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
			nextUp: (h.nextUp ?? []).map(normalizeItem),
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
	pingPlayback: (sessionId: string, body: { positionMs: number; paused: boolean }, keepalive = false) =>
		request<void>(`/playback/sessions/${encodeURIComponent(sessionId)}/ping`, { method: 'POST', body, keepalive, auth: !keepalive }),
	/**
	 * Stops a session. `keepalive` lets the request outlive the page (pagehide / unload):
	 * sendBeacon cannot send DELETE.
	 */
	stopPlayback: (sessionId: string, keepalive = false, positionMs?: number) =>
		fetch(`${API_BASE}/playback/sessions/${encodeURIComponent(sessionId)}`, {
			method: 'DELETE',
			keepalive,
			credentials: 'same-origin',
			// M6: the final position, so the server can finalise the played state in one request.
			...(positionMs !== undefined ? { headers: { 'content-type': 'application/json' }, body: JSON.stringify({ positionMs: Math.round(positionMs) }) } : {})
		}).then(
			() => undefined,
			() => undefined
		),

	// ——— Comptes (M6) ———
	setup: async (body: { name: string; password: string; device?: string }) =>
		normalizeAuth(await request<AuthResult>('/setup', { method: 'POST', body, auth: false })),
	login: async (body: { name: string; password: string; device?: string }) =>
		normalizeAuth(await request<AuthResult>('/auth/login', { method: 'POST', body, auth: false })),
	logout: () => request<void>('/auth/logout', { method: 'POST', auth: false }),
	/** Session probe: a 401 here is an answer, not a redirect. */
	me: async (signal?: AbortSignal) => normalizeUser(await request<User>('/me', { signal, auth: false })),
	patchMe: async (body: MePatch) => normalizeUser(await request<User>('/me', { method: 'PATCH', body })),
	// Session ids are 64-bit random integers: kept as strings to survive JSON.parse.
	mySessions: async (signal?: AbortSignal) => (await request<DeviceSession[]>('/me/sessions', { signal, bigIds: true })) ?? [],
	revokeSession: (id: DeviceSession['id']) => request<void>(`/me/sessions/${encodeURIComponent(String(id))}`, { method: 'DELETE' }),
	users: async (signal?: AbortSignal) => ((await request<User[]>('/users', s(signal))) ?? []).map(normalizeUser),
	createUser: async (body: UserInput) => normalizeUser(await request<User>('/users', { method: 'POST', body })),
	patchUser: async (id: number, body: UserInput) => normalizeUser(await request<User>(`/users/${id}`, { method: 'PATCH', body })),
	deleteUser: (id: number) => request<void>(`/users/${id}`, { method: 'DELETE' }),

	// ——— Progression (M6) ———
	putUserData: async (itemId: number, body: UserDataPatch) =>
		normalizeUserData(await request<UserData>(`/items/${itemId}/userdata`, { method: 'PUT', body })),

	// ——— Collections (M6) ———
	collections: async (signal?: AbortSignal) => ((await request<Collection[]>('/collections', s(signal))) ?? []).map(normalizeCollection),
	collectionItems: async (id: string, signal?: AbortSignal) => {
		const r = await request<{ collection: Collection; items: Item[] }>(`/collections/${encodeURIComponent(id)}/items`, s(signal));
		return { collection: normalizeCollection(r.collection), items: (r.items ?? []).map(normalizeItem) };
	},
	createCollection: async (body: { name: string; shared?: boolean }) =>
		normalizeCollection(await request<Collection>('/collections', { method: 'POST', body })),
	patchCollection: async (id: string, body: { name?: string; shared?: boolean }) => {
		const c = await request<Collection | undefined>(`/collections/${encodeURIComponent(id)}`, { method: 'PATCH', body });
		return c ? normalizeCollection(c) : undefined;
	},
	deleteCollection: (id: string) => request<void>(`/collections/${encodeURIComponent(id)}`, { method: 'DELETE' }),
	addToCollection: (id: string, itemId: number) => request<void>(`/collections/${encodeURIComponent(id)}/items/${itemId}`, { method: 'PUT' }),
	removeFromCollection: (id: string, itemId: number) =>
		request<void>(`/collections/${encodeURIComponent(id)}/items/${itemId}`, { method: 'DELETE' }),

	// ——— Watchlist (M6b) ———
	watchlist: async (signal?: AbortSignal) => ((await request<Item[]>('/watchlist', s(signal))) ?? []).map(normalizeItem),
	addToWatchlist: (itemId: number) => request<void>(`/watchlist/${itemId}`, { method: 'PUT' }),
	removeFromWatchlist: (itemId: number) => request<void>(`/watchlist/${itemId}`, { method: 'DELETE' }),

	// ——— Synchronisation Trakt / Simkl (M6b) ———
	mySync: async (signal?: AbortSignal) => ((await request<SyncConnection[]>('/me/sync', s(signal))) ?? []).map(normalizeSync),
	syncConnect: (service: SyncService) => request<DeviceCode>(`/me/sync/${service}/connect`, { method: 'POST' }),
	syncPoll: async (service: SyncService, signal?: AbortSignal) => {
		const r = await request<SyncPollResult>(`/me/sync/${service}/poll`, { method: 'POST', signal });
		r.connection = normalizeSync(r.connection);
		return r;
	},
	syncFeatures: async (service: SyncService, features: Partial<Record<SyncFeature, boolean>>) =>
		normalizeSync(await request<SyncConnection>(`/me/sync/${service}`, { method: 'PATCH', body: { features } })),
	syncRun: (service: SyncService, full = false) =>
		request<{ queued: boolean }>(`/me/sync/${service}/run`, { method: 'POST', body: full ? { full } : {} }),
	syncDisconnect: (service: SyncService) => request<void>(`/me/sync/${service}`, { method: 'DELETE' }),
	syncEvents: async (service: SyncService, limit = 50, signal?: AbortSignal) =>
		(await request<SyncEvent[]>(`/me/sync/${service}/events`, { params: { limit }, signal })) ?? [],
	syncApps: async (signal?: AbortSignal) => (await request<SyncApps>('/sync/apps', s(signal))) ?? {},
	setSyncApp: (service: SyncService, body: { clientId: string; clientSecret?: string }) =>
		request<SyncApp>(`/sync/apps/${service}`, { method: 'PUT', body }),
	deleteSyncApp: (service: SyncService) => request<void>(`/sync/apps/${service}`, { method: 'DELETE' }),

	// ——— Administration (M7) ———
	settings: (signal?: AbortSignal) => request<ServerSettings>('/settings', s(signal)),
	patchSettings: (body: SettingsPatch) => request<ServerSettings>('/settings', { method: 'PATCH', body }),
	/** Without apiKey: checks the key in effect. */
	checkTmdb: (apiKey?: string, signal?: AbortSignal) =>
		request<TmdbCheck>('/settings/tmdb/check', { method: 'POST', body: apiKey ? { apiKey } : {}, signal }),
	systemStatus: (detect = false, signal?: AbortSignal) => request<SystemStatus>('/system/status', { params: { detect }, signal }),
	jobQueue: async (signal?: AbortSignal) => {
		const q = await request<JobQueue>('/jobs/queue', s(signal));
		q.running ??= [];
		q.queued ??= [];
		q.failed ??= [];
		q.counts ??= { running: 0, queued: 0, failed: 0 };
		q.byKind ??= {};
		return q;
	},
	runTask: (task: AdminTask) => request<{ queued: number }>('/jobs/run', { method: 'POST', body: { task } }),
	retryJob: (id: number) => request<void>(`/jobs/${id}/retry`, { method: 'POST' }),
	cancelJob: (id: number) => request<void>(`/jobs/${id}`, { method: 'DELETE' }),
	logs: async (params: { level?: string; q?: string; after?: number; limit?: number }, signal?: AbortSignal) => {
		const p = await request<LogPage>('/logs', { params, signal });
		p.entries ??= [];
		return p;
	},
	playbackSessions: async (signal?: AbortSignal) => (await request<ActiveSession[]>('/playback/sessions', s(signal))) ?? [],
	stopPlaybackSession: (sid: string) => request<void>(`/playback/sessions/${encodeURIComponent(sid)}/stop`, { method: 'POST' })
};

/** URL of the live log stream (SSE) or of the download, with the current filters. */
export function logsUrl(kind: 'stream' | 'download', level: string, q: string): string {
	const sp = new URLSearchParams();
	if (level) sp.set('level', level);
	if (q.trim()) sp.set('q', q.trim());
	const qs = sp.toString();
	return `${API_BASE}/logs/${kind}${qs ? `?${qs}` : ''}`;
}

export function normalizeSync(c: SyncConnection): SyncConnection {
	c.supported ??= [];
	c.features ??= {};
	c.status ??= 'disconnected';
	c.syncing ??= false;
	return c;
}

export function normalizeAgent(a: Agent): Agent {
	a.status ??= 'unknown';
	a.sources ??= 0;
	a.name ||= a.url;
	a.transport ??= a.fingerprint ? 'https-pinned' : /^https:/i.test(a.url) ? 'https-ca' : 'http';
	return a;
}

function normalizeListing(l: FolderListing | undefined, path: string): FolderListing {
	return { path: l?.path ?? path, parent: l?.parent || undefined, dirs: l?.dirs ?? [] };
}

export function normalizeUser(u: User): User {
	u.libraryIds ??= [];
	u.prefs ??= {};
	u.allLibraries ??= true;
	u.blockUnrated ??= false;
	return u;
}

function normalizeAuth(r: AuthResult): AuthResult {
	if (r?.user) normalizeUser(r.user);
	return r;
}

export function normalizeUserData(d: UserData | undefined): UserData {
	return { positionMs: 0, played: false, playCount: 0, favorite: false, inWatchlist: false, ...(d ?? {}) };
}

function normalizeCollection(c: Collection): Collection {
	c.images ??= {};
	c.count ??= 0;
	c.shared ??= false;
	return c;
}
