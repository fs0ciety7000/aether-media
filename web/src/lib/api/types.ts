// Types of the HTTP API v1 — mirror of docs/API.md (the binding contract).
// Optional fields may be missing on older servers: the UI must degrade gracefully.

export type ImageType = 'poster' | 'backdrop' | 'logo' | 'thumb' | 'banner';

export interface Image {
	url: string;
	width?: number;
	height?: number;
	blurhash?: string;
	dominant?: string;
}

export type Kind = 'movie' | 'series' | 'season' | 'episode' | 'artist' | 'album' | 'track';

export type IdentifyStatus = 'pending' | 'matched' | 'unmatched' | 'manual' | 'local';

export type Resolution = '4K' | '1080p' | '720p' | 'SD';

export interface Item {
	id: number;
	libraryId: number;
	parentId?: number;
	kind: Kind;
	title: string;
	originalTitle?: string;
	year?: number;
	overview?: string;
	tagline?: string;
	runtimeMs?: number;
	indexNumber?: number;
	indexNumberEnd?: number;
	parentIndexNumber?: number;
	edition?: string;
	airDate?: string;
	premiereDate?: string;
	endDate?: string;
	status?: 'continuing' | 'ended';
	officialRating?: string;
	communityRating?: number;
	collection?: string;
	providerIds: Record<string, string>;
	path?: string;
	dateAdded: string;
	identifyStatus: IdentifyStatus;
	lockedFields: string[];
	images: Partial<Record<ImageType, Image[]>>;
	/** Seasons of a series, episodes of a season, albums, tracks. */
	childCount?: number;
	video?: { resolution: Resolution; hdr?: string };
	/** M5b: false when every file of the item sits on offline sources (greyed, playback disabled). Missing = available. */
	available?: boolean;
	/** M6: per-user data on movies, episodes and tracks. */
	userData?: UserData;
	/** M6: unwatched episodes of a series or season. */
	unplayedCount?: number;
	/**
	 * Not in the contract (yet): series of an episode, when the server provides it.
	 * The UI falls back to `ancestors` (detail) or a lookup otherwise.
	 */
	seriesTitle?: string;
	seriesId?: number;
}

export interface Person {
	id: number;
	name: string;
	kind: string;
	role?: string;
	image?: string;
}

export interface Ancestor {
	id: number;
	kind: Kind;
	title: string;
}

export interface MediaStream {
	index: number;
	type: string;
	codec: string;
	profile?: string;
	width?: number;
	height?: number;
	frameRate?: number;
	bitDepth?: number;
	hdr?: string;
	channels?: number;
	channelLayout?: string;
	language?: string;
	title?: string;
	default: boolean;
	forced: boolean;
	imageSub?: boolean;
}

export interface MediaFile {
	id: number;
	/** Administrators only (M6). */
	path?: string;
	size: number;
	part?: number;
	container?: string;
	durationMs?: number;
	bitrate?: number;
	probed: boolean;
	probeError?: string;
	streams: MediaStream[];
}

export interface ItemDetail extends Item {
	childCount: number;
	genres: string[];
	studios: string[];
	people: Person[];
	/** Root first (series, season…). */
	ancestors: Ancestor[];
	mediaFiles: MediaFile[];
}

export type LibraryKind = 'movies' | 'shows' | 'music' | 'mixed';

export interface Library {
	id: number;
	name: string;
	kind: LibraryKind;
	/** Paths of the local sources (kept for older clients). */
	paths: string[];
	counts: Record<string, number>;
	createdAt: string;
	/** M5b. Normalised by the client: derived from `paths` on servers that predate sources. */
	sources: Source[];
}

// ——— Sources (M5b) ———

export type SourceKind = 'local' | 'agent' | 'sftp';
export type SourceStatus = 'online' | 'offline' | 'unknown';
export type SftpAuth = 'password' | 'key';

export interface Source {
	id: number;
	kind: SourceKind;
	/** Display label (server default: host + folder). */
	name: string;
	/** local: absolute path · agent: sub-folder of the share ("" = root) · sftp: absolute remote path. */
	root: string;
	status: SourceStatus;
	lastError?: string;
	lastSeen?: string;
	/** M5c: `agentId` is set for sources of a paired agent (the URL then comes from the agent). */
	agent?: { agentId?: number; url: string; share: string; hostname?: string; version?: string };
	/** hostKey: "SHA256:…" fingerprint. */
	sftp?: { host: string; port: number; user: string; auth: SftpAuth; hostKey: string };
	/** Client-only: synthesised from `Library.paths` (older server) — cannot be edited or deleted. */
	derived?: boolean;
}

/** Secrets (token, password, private key) are write-only: never returned by the API. */
export interface SourceInput {
	kind: SourceKind;
	name?: string;
	root: string;
	/** Paired agent (M5c, recommended): `{ agentId, share }` · manual token: `{ url, token, share }`. */
	agent?: { agentId: number; share: string } | { url: string; token: string; share: string };
	sftp?: { host: string; port?: number; user: string; password?: string; privateKey?: string; passphrase?: string; hostKey?: string };
}

/** PATCH body: omitted secrets stay unchanged on the server. */
export interface SourcePatch {
	name?: string;
	root?: string;
	agent?: { agentId?: number; url?: string; token?: string; share?: string };
	sftp?: { host?: string; port?: number; user?: string; password?: string; privateKey?: string; passphrase?: string; hostKey?: string };
}

export interface SourceTestResult {
	ok: boolean;
	/** Machine code ("host_key_unknown", …) or a message. */
	error?: string;
	/** SFTP: fingerprint to confirm ("SHA256:…"). */
	hostKey?: string;
	agent?: { hostname: string; version: string; shares: string[] };
}

export interface SourceStatusEvent {
	id: number;
	libraryId: number;
	status: SourceStatus;
	lastError?: string;
}

// ——— Agents appairés et sélecteur de dossiers (M5c) ———

export interface Agent {
	id: number;
	uid: string;
	name: string;
	url: string;
	version?: string;
	status: SourceStatus;
	lastError?: string;
	lastSeen?: string;
	/** Number of sources using this agent (unpairing is refused while > 0). */
	sources: number;
}

/** Answer to the LAN discovery broadcast (POST /agents/discover). */
export interface DiscoveredAgent {
	uid: string;
	name: string;
	hostname: string;
	version: string;
	port: number;
	tls: boolean;
	addr: string;
	url: string;
	paired: boolean;
	agentId?: number;
}

export interface Folder {
	name: string;
	path: string;
}

/** GET /fs/browse and GET /agents/{id}/browse. Empty `path`: the roots. */
export interface FolderListing {
	path: string;
	parent?: string;
	dirs: Folder[];
}

export interface AgentShare {
	name: string;
	path: string;
	/** Created from the UI (removable); false for shares declared in agent.toml. */
	managed?: boolean;
}

export interface AgentStatusEvent {
	id: number;
	status: SourceStatus;
	lastError?: string;
}

export interface SystemInfo {
	name: string;
	version: string;
	commit: string;
	startedAt: string;
	setupRequired: boolean;
	metadata?: { tmdb: boolean; musicbrainz: boolean };
	/** M6b: synchronisation service usable (application credentials configured). */
	sync?: { trakt: boolean; simkl: boolean };
	/** M7: name chosen by the administrator (Réglages › Serveur). */
	serverName?: string;
	/** M7, administrators only: the first-run assistant was left before its end. */
	setupWizardPending?: boolean;
	/** The request carried a valid session (absent on older servers). */
	authenticated?: boolean;
}

export interface LibraryFilters {
	genres: { name: string; count: number }[];
	years: { min: number; max: number } | null;
	resolutions: string[];
	hasHdr: boolean;
}

export type SortKey = 'title' | 'dateAdded' | 'year' | 'rating' | 'runtime' | 'premiereDate' | 'random' | 'lastPlayed';
export type SortOrder = 'asc' | 'desc';

export interface ItemQuery {
	library?: number;
	parent?: number;
	kind?: Kind | Kind[];
	genre?: string;
	person?: number;
	yearFrom?: number;
	yearTo?: number;
	resolution?: Resolution;
	hdr?: boolean;
	identify?: IdentifyStatus;
	/** M6 */
	played?: boolean;
	favorite?: boolean;
	inProgress?: boolean;
	/** M6b */
	watchlist?: boolean;
	sort?: SortKey;
	order?: SortOrder;
	limit?: number;
	cursor?: string;
}

export interface ItemPage {
	items: Item[];
	nextCursor?: string;
	/** May be missing on servers that predate the M4 contract. */
	total?: number;
}

export interface Candidate {
	provider: string;
	providerIds: Record<string, string>;
	title: string;
	year?: number;
	overview?: string;
	imageUrl?: string;
	score: number;
}

export interface ItemPatch {
	title?: string;
	originalTitle?: string;
	overview?: string;
	tagline?: string;
	year?: number;
	officialRating?: string;
	unlock?: string[];
}

export interface HomeData {
	hero: Item[];
	continueWatching: Item[];
	/** M6: next unwatched episode of each started series. */
	nextUp: Item[];
	recentlyAdded: { library: { id: number; name: string; kind: string }; items: Item[] }[];
	categories: { genre: string; count: number; image?: Image }[];
	recommended: Item[];
}

export interface SearchPerson {
	id: number;
	name: string;
	image?: string;
	count: number;
}

export interface SearchResults {
	query: string;
	results: {
		movie: Item[];
		series: Item[];
		episode: Item[];
		artist: Item[];
		album: Item[];
		track: Item[];
		person: SearchPerson[];
	};
}

export type JobStats = Record<string, Record<string, number>>;

/** Server-sent event envelope (internal/events). */
export interface ServerEvent<T = unknown> {
	type: string;
	time: string;
	data?: T;
}

export interface ScanProgress {
	libraryId: number;
	processed: number;
	total: number;
}

export interface ScanResult {
	libraryId: number;
	files: number;
	added: number;
	updated: number;
	unchanged: number;
	removed: number;
	skipped: number;
	unavailable?: string[];
	durationMs: number;
}

// ——— Lecture (M5) ———

/** Capabilities detected in the browser (src/lib/player/capabilities.ts). */
export interface ClientProfile {
	/** Containers playable directly: "mp4", "webm", "mov", "ogg", "mp3", "flac", "m4a", "wav" (never "mkv"). */
	containers: string[];
	/** "h264", "hevc", "av1", "vp9", "vp8" */
	videoCodecs: string[];
	/** "aac", "mp3", "opus", "vorbis", "flac", "ac3", "eac3", "alac", "pcm" */
	audioCodecs: string[];
	/** HDR display and a 10-bit codec. */
	hdr: boolean;
	maxHeight?: number;
	/** Bits per second; 0 = unlimited. */
	maxBitrate?: number;
	hls: 'native' | 'mse';
}

export interface PlaybackTrack {
	index: number;
	language?: string;
	title?: string;
	codec: string;
	channels?: number;
	default: boolean;
	forced?: boolean;
	external?: boolean;
}

export interface SubtitleTrack extends PlaybackTrack {
	delivery: 'vtt' | 'burn';
	/** Present for delivery "vtt". */
	url?: string;
}

export type PlaybackMethod = 'direct' | 'remux' | 'transcode';

export interface PlaybackInfo {
	sessionId: string;
	method: PlaybackMethod;
	/** Why not "direct": "container", "videoCodec", "audioCodec", "bitrate", "hdr", "subtitleBurn"… */
	reasons: string[];
	/** direct: /api/v1/media/{fileId}/stream · otherwise the session's master.m3u8. */
	url: string;
	mediaFileId: number;
	durationMs: number;
	/** Actual start position (aligned on a segment in HLS). */
	startMs: number;
	audioTracks: PlaybackTrack[];
	subtitleTracks: SubtitleTrack[];
	selectedAudio: number;
	/** -1 = none. */
	selectedSubtitle: number;
	video?: { codec: string; width: number; height: number; hdr?: string; copied: boolean };
}

export interface PlaybackRequest {
	profile: ClientProfile;
	mediaFileId?: number;
	audioIndex?: number;
	subtitleIndex?: number;
	startMs?: number;
}

// ——— Comptes, progression, collections (M6) ———

export type SubtitleMode = 'auto' | 'always' | 'forced' | 'off';

export interface UserPrefs {
	audioLanguage?: string;
	subtitleLanguage?: string;
	subtitleMode?: SubtitleMode;
}

/** Parental limit: 0, 6, 10, 12, 16, 18; missing = none. */
export type RatingAge = 0 | 6 | 10 | 12 | 16 | 18;

export interface User {
	id: number;
	name: string;
	isAdmin: boolean;
	allLibraries: boolean;
	/** Access when !allLibraries. */
	libraryIds: number[];
	maxRatingAge?: RatingAge;
	blockUnrated: boolean;
	prefs: UserPrefs;
	createdAt: string;
	lastSeen?: string;
}

export interface UserData {
	positionMs: number;
	played: boolean;
	playCount: number;
	lastPlayedAt?: string;
	favorite: boolean;
	/** 0–10 */
	rating?: number;
	/** M6b: in the user's watchlist ("Ma liste"). Movies and series only. */
	inWatchlist?: boolean;
}

export interface AuthResult {
	user: User;
	token: string;
}

export interface DeviceSession {
	/** 64-bit on the server: a string when beyond Number.MAX_SAFE_INTEGER (see api.mySessions). */
	id: number | string;
	device: string;
	client: string;
	current: boolean;
	lastSeen: string;
	createdAt: string;
}

export interface MePatch {
	prefs?: UserPrefs;
	currentPassword?: string;
	newPassword?: string;
}

export interface UserInput {
	name?: string;
	/** Create, or reset on PATCH. */
	password?: string;
	isAdmin?: boolean;
	allLibraries?: boolean;
	libraryIds?: number[];
	/** null on PATCH removes the limit. */
	maxRatingAge?: RatingAge | null;
	blockUnrated?: boolean;
}

export interface UserDataPatch {
	played?: boolean;
	favorite?: boolean;
	/** null clears the rating (M6b). */
	rating?: number | null;
	positionMs?: number;
}

export interface Collection {
	/** "c<id>" manual · "t<name>" automatic (TMDB). */
	id: string;
	name: string;
	kind: 'manual' | 'tmdb';
	count: number;
	/** Poster/backdrop of the first item when the collection has none. */
	images: Item['images'];
	ownerId?: number;
	shared: boolean;
}

// ——— Synchronisation Trakt / Simkl (M6b) ———

export type SyncService = 'trakt' | 'simkl';
export type SyncFeature = 'scrobble' | 'history' | 'progress' | 'watchlist' | 'ratings';
export type SyncStatus = 'disconnected' | 'pending' | 'connected' | 'reauth_required';

export interface DeviceCode {
	/** Shown large (e.g. "A1B2C3D4"). */
	userCode: string;
	verificationUrl: string;
	/** ISO 8601. */
	expiresAt: string;
	/** Minimal seconds between two /poll calls. */
	interval: number;
}

export interface SyncConnection {
	service: SyncService;
	/** Application credentials configured by an administrator. */
	available: boolean;
	status: SyncStatus;
	account?: { username: string };
	supported: SyncFeature[];
	/** Only the supported features. */
	features: Partial<Record<SyncFeature, boolean>>;
	/** A synchronisation is queued or running. */
	syncing: boolean;
	connectedAt?: string;
	lastSyncAt?: string;
	/** User-facing text, no secret. */
	lastError?: string;
	lastErrorAt?: string;
	/** Pairing in progress (status "pending", or a reconnection of an existing account). */
	pending?: DeviceCode;
}

export type SyncPollStatus = 'pending' | 'slow_down' | 'connected' | 'expired' | 'denied';

export interface SyncPollResult {
	status: SyncPollStatus;
	interval: number;
	connection: SyncConnection;
}

export interface SyncEvent {
	id: number;
	time: string;
	kind: 'conflict' | 'error' | 'import' | 'export';
	feature: SyncFeature;
	itemId?: number;
	itemTitle?: string;
	/** Conflicts: side kept (most recent date). */
	winner?: 'local' | 'remote';
	message: string;
}

export interface SyncApp {
	configured: boolean;
	source: 'config' | 'database' | 'none';
	/** Masked ("abcd…wxyz"). */
	clientId?: string;
}

export type SyncApps = Partial<Record<SyncService, SyncApp>>;

/** SSE `sync.status`. */
export interface SyncStatusEvent {
	userId: number;
	service: SyncService;
	status: SyncStatus;
	syncing: boolean;
	lastError?: string;
}

/** SSE `userdata.changed`. */
export interface UserDataChangedEvent {
	userId: number;
	itemId: number;
}

// ——— Administration (M7) ———

/** Where a setting comes from: configuration file, AETHER_* variable, this UI, or the default. */
export type SettingSource = 'file' | 'env' | 'database' | 'default';

export interface SettingField<T> {
	value: T;
	source: SettingSource;
	/** Defined by the configuration: shown, never editable here. */
	readOnly: boolean;
	/** When a change takes effect. */
	applies: 'live' | 'restart';
	/** Saved, but the server still runs with the previous value. */
	restartRequired?: boolean;
}

export interface SecretSetting {
	set: boolean;
	/** "…ab12": never the full value. */
	masked?: string;
	source: SettingSource;
	readOnly: boolean;
	applies: 'live' | 'restart';
}

export type HWAccelMode = 'none' | 'auto' | 'vaapi' | 'qsv' | 'nvenc' | 'videotoolbox';
export interface HWAccel {
	mode: HWAccelMode;
	device?: string;
}

export type WizardStep = 'account' | 'metadata' | 'tmdb' | 'library' | 'summary' | 'done';
export interface WizardState {
	step: WizardStep;
	completed: boolean;
	skipped?: WizardStep[];
}

export interface ServerSettings {
	serverName: SettingField<string>;
	metadataLanguage: SettingField<string>;
	/** "" = deduced from the language. */
	metadataCountry: SettingField<string>;
	tmdbApiKey: SecretSetting;
	hwAccel: SettingField<HWAccel>;
	maxTranscodes: SettingField<number>;
	/** 0 = periodic rescans disabled. */
	scanIntervalMinutes: SettingField<number>;
	wizard: WizardState;
}

export interface SettingsPatch {
	serverName?: string;
	metadataLanguage?: string;
	metadataCountry?: string;
	/** "" removes the stored key. */
	tmdbApiKey?: string;
	hwAccel?: HWAccel;
	maxTranscodes?: number;
	scanIntervalMinutes?: number;
	wizard?: { step?: WizardStep; completed?: boolean; skipped?: WizardStep[] };
}

export interface TmdbCheck {
	valid: boolean;
	error?: 'invalid_key' | 'unreachable' | 'missing';
	message?: string;
}

export interface BinaryStatus {
	path: string;
	ok: boolean;
	version?: string;
	error?: string;
}

export interface SystemStatus {
	name: string;
	version: string;
	commit: string;
	goVersion: string;
	os: string;
	arch: string;
	startedAt: string;
	paths: { data: string; cache: string; transcode: string; database: string; databaseSize: number };
	ffmpeg: BinaryStatus;
	ffprobe: BinaryStatus;
	hwAccel: { configured: HWAccelMode; device?: string; active: string; detected: string[]; checkedAt: string };
	transcodes: { max: number; active: number };
	logs: { capacity: number; held: number };
}

export type JobStatus = 'running' | 'queued' | 'failed';
export interface Job {
	id: number;
	kind: string;
	key: string;
	label?: string;
	status: JobStatus;
	priority: number;
	attempts: number;
	error?: string;
	runAfter: string;
	createdAt: string;
	updatedAt: string;
}

export interface JobQueue {
	running: Job[];
	queued: Job[];
	failed: Job[];
	counts: Record<JobStatus, number>;
	byKind: Record<string, Partial<Record<JobStatus, number>>>;
}

export type AdminTask = 'scan' | 'metadata' | 'sync';

export type LogLevel = 'DEBUG' | 'INFO' | 'WARN' | 'ERROR';
export interface LogEntry {
	seq: number;
	time: string;
	level: LogLevel;
	msg: string;
	attrs?: { k: string; v: string }[];
}

export interface LogPage {
	entries: LogEntry[];
	held: number;
	capacity: number;
}

export interface ActiveSession {
	id: string;
	user?: { id: number; name: string };
	item?: { id: number; kind: Kind; title: string; seriesTitle?: string; season?: number; episode?: number; year?: number };
	method: 'direct' | 'remux' | 'transcode';
	reasons: string[];
	positionMs: number;
	durationMs: number;
	paused: boolean;
	/** bits/s (estimate). */
	bitrate: number;
	video?: { codec: string; copied: boolean; width?: number; height?: number };
	audio?: { codec: string; copied: boolean };
	hwAccel?: string;
	startedAt: string;
	lastActive: string;
}
