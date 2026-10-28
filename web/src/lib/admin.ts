// Administration (M7): French labels for server settings, jobs, logs and playback sessions.
import type { HWAccelMode, JobStatus, LogLevel, SettingSource, WizardStep } from '$lib/api/types';

/** Metadata languages offered by TMDB (BCP-47), in French. */
export const METADATA_LANGUAGES: { value: string; label: string }[] = [
	{ value: 'fr-FR', label: 'Français (France)' },
	{ value: 'fr-CA', label: 'Français (Canada)' },
	{ value: 'en-US', label: 'Anglais (États-Unis)' },
	{ value: 'en-GB', label: 'Anglais (Royaume-Uni)' },
	{ value: 'de-DE', label: 'Allemand' },
	{ value: 'es-ES', label: 'Espagnol (Espagne)' },
	{ value: 'es-MX', label: 'Espagnol (Mexique)' },
	{ value: 'it-IT', label: 'Italien' },
	{ value: 'nl-NL', label: 'Néerlandais' },
	{ value: 'pt-PT', label: 'Portugais (Portugal)' },
	{ value: 'pt-BR', label: 'Portugais (Brésil)' },
	{ value: 'pl-PL', label: 'Polonais' },
	{ value: 'sv-SE', label: 'Suédois' },
	{ value: 'ja-JP', label: 'Japonais' },
	{ value: 'ko-KR', label: 'Coréen' }
];

/** Countries for age ratings and release dates (ISO 3166-1). */
export const COUNTRIES: { value: string; label: string }[] = [
	{ value: 'FR', label: 'France' },
	{ value: 'BE', label: 'Belgique' },
	{ value: 'CH', label: 'Suisse' },
	{ value: 'LU', label: 'Luxembourg' },
	{ value: 'CA', label: 'Canada' },
	{ value: 'US', label: 'États-Unis' },
	{ value: 'GB', label: 'Royaume-Uni' },
	{ value: 'DE', label: 'Allemagne' },
	{ value: 'ES', label: 'Espagne' },
	{ value: 'IT', label: 'Italie' },
	{ value: 'NL', label: 'Pays-Bas' },
	{ value: 'PT', label: 'Portugal' },
	{ value: 'BR', label: 'Brésil' },
	{ value: 'MX', label: 'Mexique' },
	{ value: 'PL', label: 'Pologne' },
	{ value: 'SE', label: 'Suède' },
	{ value: 'JP', label: 'Japon' },
	{ value: 'KR', label: 'Corée du Sud' }
];

export function languageLabel(code: string | undefined): string {
	return METADATA_LANGUAGES.find((l) => l.value === code)?.label ?? code ?? '';
}

export function countryLabel(code: string | undefined): string {
	return COUNTRIES.find((c) => c.value === code)?.label ?? code ?? '';
}

/** "fr-FR" → "FR". */
export function regionOf(lang: string | undefined): string {
	const r = lang?.split('-')[1];
	return r && r.length === 2 ? r.toUpperCase() : '';
}

export const SOURCE_LABEL: Record<SettingSource, string> = {
	file: 'Fichier de configuration',
	env: 'Variable d’environnement',
	database: 'Réglé ici',
	default: 'Valeur par défaut'
};

/** Environment variable that overrides each setting (shown on read-only values). */
export const SETTING_ENV: Record<string, string> = {
	serverName: 'AETHER_SERVER_NAME',
	metadataLanguage: 'AETHER_LANGUAGE',
	metadataCountry: 'AETHER_METADATA_COUNTRY',
	tmdbApiKey: 'AETHER_TMDB_API_KEY',
	hwAccel: 'AETHER_HWACCEL',
	maxTranscodes: 'AETHER_MAX_TRANSCODES',
	scanIntervalMinutes: 'AETHER_SCAN_INTERVAL'
};

export const HW_MODES: { value: HWAccelMode; label: string; hint: string }[] = [
	{ value: 'none', label: 'Aucune (logiciel)', hint: 'Le processeur encode la vidéo. Toujours disponible, plus gourmand.' },
	{ value: 'auto', label: 'Automatique', hint: 'Aether essaie les encodeurs matériels au démarrage et garde le premier qui fonctionne.' },
	{ value: 'vaapi', label: 'VAAPI (Intel, AMD sous Linux)', hint: 'Nécessite l’accès au périphérique de rendu, par exemple /dev/dri/renderD128.' },
	{ value: 'qsv', label: 'Intel Quick Sync (QSV)', hint: 'Processeurs Intel récents, sous Linux ou Windows.' },
	{ value: 'nvenc', label: 'NVIDIA NVENC', hint: 'Carte NVIDIA avec pilote propriétaire.' },
	{ value: 'videotoolbox', label: 'VideoToolbox (macOS)', hint: 'Encodeur matériel d’Apple.' }
];

export function hwLabel(mode: string | undefined): string {
	if (!mode || mode === 'none') return 'Logiciel';
	return HW_MODES.find((m) => m.value === mode)?.label.replace(/ \(.*\)$/, '') ?? mode;
}

/** Encoder name reported by FFmpeg → mode label. */
export function encoderLabel(enc: string): string {
	return { h264_vaapi: 'VAAPI', h264_qsv: 'Quick Sync', h264_nvenc: 'NVENC', h264_videotoolbox: 'VideoToolbox' }[enc] ?? enc;
}

/** Periodic rescan choices (minutes, 0 = disabled). */
export const SCAN_INTERVALS: { value: number; label: string }[] = [
	{ value: 0, label: 'Désactivé (surveillance des dossiers seulement)' },
	{ value: 60, label: 'Toutes les heures' },
	{ value: 180, label: 'Toutes les 3 heures' },
	{ value: 360, label: 'Toutes les 6 heures' },
	{ value: 720, label: 'Toutes les 12 heures' },
	{ value: 1440, label: 'Une fois par jour' },
	{ value: 10080, label: 'Une fois par semaine' }
];

export function intervalLabel(min: number): string {
	const known = SCAN_INTERVALS.find((i) => i.value === min);
	if (known) return known.label;
	if (min % 60 === 0) return `Toutes les ${min / 60} heures`;
	return `Toutes les ${min} minutes`;
}

export const WIZARD_STEPS: { id: WizardStep; label: string }[] = [
	{ id: 'account', label: 'Compte administrateur' },
	{ id: 'metadata', label: 'Langue et pays' },
	{ id: 'tmdb', label: 'Clé TMDB' },
	{ id: 'library', label: 'Première bibliothèque' },
	{ id: 'summary', label: 'Récapitulatif' }
];

// ——— Jobs ———

const JOB_KINDS: Record<string, string> = {
	'library.scan': 'Analyse de bibliothèque',
	'media.probe': 'Analyse technique',
	'metadata.refresh': 'Métadonnées',
	'image.fetch': 'Image',
	'sync.run': 'Synchronisation',
	'sync.scrobble': 'Scrobbling'
};

export function jobKindLabel(kind: string): string {
	return JOB_KINDS[kind] ?? kind;
}

export const JOB_STATUS_LABEL: Record<JobStatus, string> = {
	running: 'En cours',
	queued: 'En attente',
	failed: 'En échec'
};

// ——— Logs ———

export const LOG_LEVELS: { value: string; label: string }[] = [
	{ value: 'debug', label: 'Tout' },
	{ value: 'info', label: 'Infos' },
	{ value: 'warn', label: 'Avertissements' },
	{ value: 'error', label: 'Erreurs' }
];

export const LOG_LEVEL_LABEL: Record<LogLevel, string> = {
	DEBUG: 'Débogage',
	INFO: 'Info',
	WARN: 'Avert.',
	ERROR: 'Erreur'
};

// ——— Playback ———
// Method and reason labels are shared with the player (lib/player/labels.ts).
export { METHOD_LABEL, reasonLabel } from '$lib/player/labels';
