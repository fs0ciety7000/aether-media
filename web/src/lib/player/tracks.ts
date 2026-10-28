// Default audio / subtitle tracks from the user's preferences (GET /me → prefs), used when the
// viewer has not picked a track explicitly (?audio / ?sub).
import type { SubtitleMode, UserPrefs } from '$lib/api/types';

export interface TrackLike {
	index: number;
	language?: string;
	title?: string;
	default?: boolean;
	forced?: boolean;
}

/** ISO 639-1 / 639-2 (B and T) spellings collapse to one key: fre = fra = fr. */
const ALIASES: Record<string, string> = {
	fre: 'fr', fra: 'fr', eng: 'en', ger: 'de', deu: 'de', spa: 'es', ita: 'it', jpn: 'ja', por: 'pt', dut: 'nl', nld: 'nl',
	rus: 'ru', chi: 'zh', zho: 'zh', kor: 'ko', ara: 'ar', swe: 'sv', dan: 'da', nor: 'no', nob: 'no', fin: 'fi', pol: 'pl',
	tur: 'tr', cze: 'cs', ces: 'cs', gre: 'el', ell: 'el', heb: 'he', hin: 'hi', hun: 'hu', ukr: 'uk'
};

export function langKey(code: string | undefined | null): string | undefined {
	if (!code) return undefined;
	const c = code.trim().toLowerCase().split(/[-_]/)[0];
	if (!c || c === 'und' || c === 'zxx' || c === 'mul') return undefined;
	return ALIASES[c] ?? c;
}

export function sameLanguage(a: string | undefined | null, b: string | undefined | null): boolean {
	const ka = langKey(a);
	return !!ka && ka === langKey(b);
}

/** A "forced" track: flagged by the container, or titled so ("Forced", "Forcés"). */
function isForced(t: TrackLike): boolean {
	return !!t.forced || /\bforc/i.test(t.title ?? '');
}

/** Audio track in the preferred language (default flag breaks ties); undefined = keep the server's choice. */
export function pickAudio<T extends TrackLike>(tracks: T[], prefs: UserPrefs | undefined): T | undefined {
	const want = prefs?.audioLanguage;
	if (!want) return undefined;
	const matches = tracks.filter((t) => sameLanguage(t.language, want));
	return matches.find((t) => t.default) ?? matches[0];
}

/**
 * Subtitle track for the audio being played, per `subtitleMode`:
 * - off: none;
 * - always: the preferred subtitle language (full track first, forced as a last resort);
 * - forced: forced track in the audio language (else in the subtitle language);
 * - auto (default): full subtitles when the audio is not in the preferred subtitle language,
 *   otherwise only forced ones.
 * Returns -1 for "none".
 */
export function pickSubtitle<T extends TrackLike>(tracks: T[], prefs: UserPrefs | undefined, audioLanguage: string | undefined): number {
	const mode: SubtitleMode = prefs?.subtitleMode ?? 'auto';
	if (mode === 'off' || !tracks.length) return -1;
	const subLang = prefs?.subtitleLanguage ?? prefs?.audioLanguage;
	const full = (lang: string | undefined) => tracks.filter((t) => sameLanguage(t.language, lang) && !isForced(t));
	const forced = (lang: string | undefined) => tracks.filter((t) => sameLanguage(t.language, lang) && isForced(t));
	const first = (list: T[]) => (list.find((t) => t.default) ?? list[0])?.index ?? -1;

	if (mode === 'always') {
		const f = full(subLang);
		return f.length ? first(f) : first(forced(subLang));
	}
	const forcedForAudio = () => {
		const a = forced(audioLanguage);
		return a.length ? first(a) : first(forced(subLang));
	};
	if (mode === 'forced') return forcedForAudio();

	// auto
	if (subLang && audioLanguage && !sameLanguage(audioLanguage, subLang)) {
		const f = full(subLang);
		if (f.length) return first(f);
	}
	if (subLang && !audioLanguage) {
		// Unknown audio language: do not guess, forced tracks only.
		return first(forced(subLang));
	}
	return forcedForAudio();
}
