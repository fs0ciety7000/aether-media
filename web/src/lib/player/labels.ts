// French labels for the player: methods, reasons, tracks, errors.
import { ApiError } from '$lib/api/client';
import type { PlaybackMethod, PlaybackTrack } from '$lib/api/types';
import { channelsLabel, codecName, languageName } from '$lib/media';

export const METHOD_LABEL: Record<PlaybackMethod, string> = {
	direct: 'Lecture directe',
	remux: 'Remux',
	transcode: 'Transcodage'
};

const REASONS: Record<string, string> = {
	container: 'conteneur non pris en charge',
	videoCodec: 'codec vidéo non pris en charge',
	audioCodec: 'codec audio non pris en charge',
	bitrate: 'débit supérieur à la limite',
	resolution: 'résolution supérieure à la limite',
	height: 'résolution supérieure à la limite',
	hdr: 'HDR non affichable (conversion SDR)',
	subtitleBurn: 'sous-titres incrustés',
	audioTrack: 'piste audio non par défaut',
	audioChannels: 'nombre de canaux audio',
	bitDepth: 'profondeur de couleur',
	profile: 'profil vidéo non pris en charge',
	level: 'niveau vidéo non pris en charge'
};

export function reasonLabel(code: string): string {
	return REASONS[code] ?? code;
}

/** Short codec label for track lists: "AC3", "E-AC3", "AAC"… */
const SHORT: Record<string, string> = { ac3: 'AC3', eac3: 'E-AC3', truehd: 'TrueHD', dts: 'DTS', aac: 'AAC', opus: 'Opus', flac: 'FLAC', mp3: 'MP3', vorbis: 'Vorbis' };

function channels(n?: number): string | undefined {
	return n ? channelsLabel({ channels: n }) : undefined;
}

/** "Français · 5.1 AC3" (+ title when it adds something, e.g. "Commentaires"). */
export function audioTrackLabel(t: PlaybackTrack): { main: string; detail: string } {
	const lang = languageName(t.language) ?? `Piste ${t.index}`;
	const codec = SHORT[t.codec?.toLowerCase()] ?? codecName(t.codec);
	const detail = [channels(t.channels), codec].filter(Boolean).join(' ');
	const main = t.title && !t.title.toLowerCase().includes((languageName(t.language) ?? '###').toLowerCase()) ? `${lang} · ${t.title}` : lang;
	return { main, detail };
}

export function subtitleTrackLabel(t: PlaybackTrack): { main: string; detail: string } {
	const lang = languageName(t.language) ?? `Sous-titres ${t.index}`;
	const flags = [t.forced ? 'forcés' : undefined, t.title && !/^(forced|forcés?)$/i.test(t.title) ? t.title : undefined].filter(Boolean).join(', ');
	return { main: flags ? `${lang} (${flags})` : lang, detail: codecName(t.codec) + (t.external ? ' · externe' : '') };
}

/** French message for a playback failure. */
export function playbackErrorMessage(err: unknown): string {
	if (err instanceof ApiError) {
		if (err.code === 'transcode_busy' || err.status === 503) return 'Trop de lectures en cours sur le serveur, réessayez dans un instant.';
		if (err.status === 404) return 'Ce titre ou son fichier est introuvable sur le serveur.';
		if (err.status === 422 || err.code === 'no_media') return 'Aucun fichier lisible pour ce titre.';
		return err.message;
	}
	if (err instanceof PlayerError) return err.message;
	if (err instanceof TypeError) return 'Le serveur est injoignable.';
	return 'Une erreur inattendue est survenue pendant la lecture.';
}

/** Failure raised by the media engine (decode, network, unsupported source). */
export class PlayerError extends Error {
	readonly kind: 'network' | 'media' | 'unsupported' | 'other';
	constructor(kind: PlayerError['kind'], message: string) {
		super(message);
		this.name = 'PlayerError';
		this.kind = kind;
	}
}
