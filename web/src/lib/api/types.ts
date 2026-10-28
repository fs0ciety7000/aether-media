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
	agent?: { url: string; share: string; hostname?: string; version?: string };
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
	agent?: { url: string; token: string; share: string };
	sftp?: { host: string; port?: number; user: string; password?: string; privateKey?: string; passphrase?: string; hostKey?: string };
}

/** PATCH body: omitted secrets stay unchanged on the server. */
export interface SourcePatch {
	name?: string;
	root?: string;
	agent?: { url?: string; token?: string; share?: string };
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

export interface SystemInfo {
	name: string;
	version: string;
	commit: string;
	startedAt: string;
	setupRequired: boolean;
	metadata?: { tmdb: boolean; musicbrainz: boolean };
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
	rating?: number;
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
