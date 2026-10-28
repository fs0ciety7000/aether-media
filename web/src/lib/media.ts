// Media vocabulary shared by views: labels, icons, technical descriptions.
import type { IconName } from '$lib/components/Icon.svelte';
import type { Item, Kind, LibraryKind, MediaStream } from '$lib/api/types';

export const LIBRARY_KINDS: { id: LibraryKind; label: string; icon: IconName; hint: string }[] = [
	{ id: 'movies', label: 'Films', icon: 'film', hint: 'Un dossier par film, « Titre (Année) ». Les NFO et affiches locales sont lus.' },
	{ id: 'shows', label: 'Séries', icon: 'tv', hint: 'Série / Saison / épisodes nommés S01E01.' },
	{ id: 'music', label: 'Musique', icon: 'music', hint: 'Artiste / Album / pistes ; les étiquettes des fichiers priment.' },
	{ id: 'mixed', label: 'Mixte', icon: 'layers', hint: 'Films et séries mêlés : chaque dossier est reconnu individuellement.' }
];

export function libraryKindInfo(kind: string) {
	return LIBRARY_KINDS.find((k) => k.id === kind) ?? LIBRARY_KINDS[3];
}

export const KIND_LABEL: Record<Kind, { one: string; many: string }> = {
	movie: { one: 'film', many: 'films' },
	series: { one: 'série', many: 'séries' },
	season: { one: 'saison', many: 'saisons' },
	episode: { one: 'épisode', many: 'épisodes' },
	artist: { one: 'artiste', many: 'artistes' },
	album: { one: 'album', many: 'albums' },
	track: { one: 'piste', many: 'pistes' }
};

export const KIND_ICON: Record<Kind, IconName> = {
	movie: 'film',
	series: 'tv',
	season: 'tv',
	episode: 'tv',
	artist: 'user',
	album: 'disc',
	track: 'music'
};

/** Square artwork (covers, artists) vs 2:3 posters. */
export function isSquare(kind: Kind): boolean {
	return kind === 'artist' || kind === 'album' || kind === 'track';
}

export function itemHref(item: Pick<Item, 'id' | 'kind' | 'parentId'>): string {
	// A track has no page of its own: show it in its album.
	if (item.kind === 'track' && item.parentId) return `/item/${item.parentId}`;
	return `/item/${item.id}`;
}

/** "S2 · É4" for episodes. */
export function episodeCode(it: Pick<Item, 'parentIndexNumber' | 'indexNumber' | 'indexNumberEnd'>): string | undefined {
	if (it.indexNumber === undefined) return undefined;
	const e = it.indexNumberEnd && it.indexNumberEnd !== it.indexNumber ? `${it.indexNumber}–${it.indexNumberEnd}` : `${it.indexNumber}`;
	return it.parentIndexNumber !== undefined ? `S${it.parentIndexNumber} · É${e}` : `Épisode ${e}`;
}

const LANGS: Record<string, string> = {
	fre: 'Français', fra: 'Français', fr: 'Français', eng: 'Anglais', en: 'Anglais', ger: 'Allemand', deu: 'Allemand',
	de: 'Allemand', spa: 'Espagnol', es: 'Espagnol', ita: 'Italien', it: 'Italien', jpn: 'Japonais', ja: 'Japonais',
	por: 'Portugais', pt: 'Portugais', dut: 'Néerlandais', nld: 'Néerlandais', nl: 'Néerlandais', rus: 'Russe',
	ru: 'Russe', chi: 'Chinois', zho: 'Chinois', zh: 'Chinois', kor: 'Coréen', ko: 'Coréen', ara: 'Arabe', ar: 'Arabe',
	swe: 'Suédois', sv: 'Suédois', dan: 'Danois', da: 'Danois', nor: 'Norvégien', no: 'Norvégien', fin: 'Finnois',
	fi: 'Finnois', pol: 'Polonais', pl: 'Polonais', tur: 'Turc', tr: 'Turc', und: 'Indéterminée'
};

export function languageName(code: string | undefined): string | undefined {
	if (!code) return undefined;
	return LANGS[code.toLowerCase()] ?? code.toUpperCase();
}

const CODECS: Record<string, string> = {
	h264: 'H.264', hevc: 'HEVC', h265: 'HEVC', av1: 'AV1', vp9: 'VP9', vp8: 'VP8', mpeg2video: 'MPEG-2', mpeg4: 'MPEG-4',
	aac: 'AAC', ac3: 'Dolby Digital', eac3: 'Dolby Digital+', truehd: 'Dolby TrueHD', dts: 'DTS', flac: 'FLAC',
	opus: 'Opus', vorbis: 'Vorbis', mp3: 'MP3', pcm_s16le: 'PCM', pcm_s24le: 'PCM', alac: 'ALAC',
	subrip: 'SRT', ass: 'ASS', ssa: 'SSA', webvtt: 'WebVTT', hdmv_pgs_subtitle: 'PGS', dvd_subtitle: 'VobSub', mov_text: 'TX3G'
};

export function codecName(codec: string | undefined): string {
	if (!codec) return '—';
	return CODECS[codec.toLowerCase()] ?? codec.toUpperCase();
}

export function resolutionLabel(width?: number, height?: number): string | undefined {
	if (!width || !height) return undefined;
	if (width >= 3200 || height >= 1800) return '4K';
	if (width >= 1700 || height >= 1000) return '1080p';
	if (width >= 1100 || height >= 650) return '720p';
	return 'SD';
}

export function channelsLabel(s: Pick<MediaStream, 'channels' | 'channelLayout'>): string | undefined {
	if (!s.channels) return s.channelLayout;
	if (s.channels === 1) return 'Mono';
	if (s.channels === 2) return 'Stéréo';
	if (s.channels === 6) return '5.1';
	if (s.channels === 8) return '7.1';
	return `${s.channels} canaux`;
}

/** Deterministic hue (0–360) from a string, for generated placeholders. */
export function hueFrom(text: string): number {
	let h = 0;
	for (let i = 0; i < text.length; i++) h = (h * 31 + text.charCodeAt(i)) >>> 0;
	// Stay in calm, cinematic hues (blues, teals, violets, warm ambers), skip neon greens.
	const palette = [196, 208, 222, 236, 252, 268, 286, 18, 32, 172, 184, 344];
	return palette[h % palette.length];
}

/** M5b: false only when the server says every file is on an offline source. */
export function isOffline(item: { available?: boolean } | undefined | null): boolean {
	return item?.available === false;
}

export const OFFLINE_HINT = 'Source hors ligne';

/** Languages offered in the playback preferences (ISO 639-2/B, as ffprobe reports them). */
export const PREF_LANGUAGES: { value: string; label: string }[] = [
	'fre', 'eng', 'ger', 'spa', 'ita', 'por', 'dut', 'jpn', 'kor', 'chi', 'rus', 'ara', 'swe', 'dan', 'nor', 'fin', 'pol', 'tur'
].map((c) => ({ value: c, label: LANGS[c] }));

export const SUBTITLE_MODES: { value: 'auto' | 'always' | 'forced' | 'off'; label: string; hint: string }[] = [
	{ value: 'auto', label: 'Automatique', hint: 'Sous-titres quand la langue audio diffère de votre langue de sous-titres ; sinon, seulement les passages forcés.' },
	{ value: 'always', label: 'Toujours', hint: 'Sous-titres dans votre langue dès qu’ils existent.' },
	{ value: 'forced', label: 'Forcés seulement', hint: 'Uniquement les passages traduits (dialogues en langue étrangère, panneaux).' },
	{ value: 'off', label: 'Désactivés', hint: 'Jamais de sous-titres par défaut ; vous pouvez toujours en choisir pendant la lecture.' }
];
