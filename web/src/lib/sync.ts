// Synchronisation Trakt / Simkl (M6b): UI copy shared by the profile and the settings.
import type { IconName } from '$lib/components/Icon.svelte';
import type { SyncEvent, SyncFeature, SyncService } from '$lib/api/types';

export const SYNC_SERVICES: SyncService[] = ['trakt', 'simkl'];

export const SERVICE_INFO: Record<SyncService, { name: string; site: string; pitch: string }> = {
	trakt: {
		name: 'Trakt',
		site: 'trakt.tv',
		pitch: 'Historique, progression de reprise, watchlist et notes, avec le suivi de lecture en direct.'
	},
	simkl: {
		name: 'Simkl',
		site: 'simkl.com',
		pitch: 'Historique, liste « Plan to Watch » et notes ; les titres finis sont ajoutés à l’historique.'
	}
};

/** Display order and copy of the features (only the supported ones are shown). */
export const FEATURES: { key: SyncFeature; label: string; hint: Record<SyncService, string> }[] = [
	{
		key: 'scrobble',
		label: 'Suivi de lecture',
		hint: {
			trakt: 'Signale en direct ce que vous regardez (lecture, pause, fin) ; un titre terminé compte comme vu.',
			simkl: 'Ajoute le titre à votre historique Simkl quand vous le terminez.'
		}
	},
	{
		key: 'history',
		label: 'Historique vu',
		hint: {
			trakt: 'Garde « vu / non vu » identique des deux côtés, films et épisodes.',
			simkl: 'Garde « vu / non vu » identique des deux côtés, films et épisodes.'
		}
	},
	{
		key: 'progress',
		label: 'Progression de reprise',
		hint: {
			trakt: 'Reprenez ici un film ou un épisode commencé ailleurs, et inversement.',
			simkl: 'Reprenez ici un film ou un épisode commencé ailleurs, et inversement.'
		}
	},
	{
		key: 'watchlist',
		label: 'Watchlist',
		hint: {
			trakt: 'Synchronise « Ma liste » avec votre watchlist Trakt.',
			simkl: 'Synchronise « Ma liste » avec votre liste « Plan to Watch ».'
		}
	},
	{
		key: 'ratings',
		label: 'Notes',
		hint: {
			trakt: 'Vos notes sur 10, arrondies à l’entier côté Trakt.',
			simkl: 'Vos notes sur 10, arrondies à l’entier côté Simkl.'
		}
	}
];

export const FEATURE_LABEL: Record<SyncFeature, string> = Object.fromEntries(FEATURES.map((f) => [f.key, f.label])) as Record<SyncFeature, string>;

export const EVENT_KIND: Record<SyncEvent['kind'], { label: string; icon: IconName }> = {
	conflict: { label: 'Conflit', icon: 'sliders' },
	import: { label: 'Import', icon: 'download' },
	export: { label: 'Export', icon: 'upload' },
	error: { label: 'Erreur', icon: 'alert' }
};

export function winnerLabel(e: SyncEvent, service: SyncService): string | undefined {
	if (e.kind !== 'conflict' || !e.winner) return undefined;
	return e.winner === 'local' ? 'Aether retenu' : `${SERVICE_INFO[service].name} retenu`;
}

/** "A1B2C3D4" → "A1B2 C3D4" for reading (the raw code is what gets copied). */
export function groupCode(code: string): string {
	const c = code.trim();
	if (c.length < 6 || /[\s-]/.test(c)) return c;
	const half = Math.ceil(c.length / 2);
	return `${c.slice(0, half)} ${c.slice(half)}`;
}

/** "4:59" from milliseconds (never negative). */
export function formatCountdown(ms: number): string {
	const s = Math.max(0, Math.ceil(ms / 1000));
	return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
}

/** Redirect URI asked by trakt.tv / simkl.com (device code / PIN flows never redirect). */
export const OOB_REDIRECT = 'urn:ietf:wg:oauth:2.0:oob';
