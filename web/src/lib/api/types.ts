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
	path: string;
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
	paths: string[];
	counts: Record<string, number>;
	createdAt: string;
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

export type SortKey = 'title' | 'dateAdded' | 'year' | 'rating' | 'runtime' | 'premiereDate' | 'random';
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
