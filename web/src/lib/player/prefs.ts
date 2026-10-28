// Player preferences kept in localStorage (per browser).
import type { QualityChoice } from './capabilities';

export type SubtitleSize = 's' | 'm' | 'l';

const KEYS = { volume: 'aether.player.volume', muted: 'aether.player.muted', quality: 'aether.player.quality', subSize: 'aether.player.subtitleSize', stats: 'aether.player.stats' };

function get(key: string): string | null {
	try {
		return localStorage.getItem(key);
	} catch {
		return null;
	}
}

function set(key: string, value: string) {
	try {
		localStorage.setItem(key, value);
	} catch {
		/* not persisted */
	}
}

export const prefs = {
	volume(): number {
		const v = Number(get(KEYS.volume));
		return get(KEYS.volume) !== null && Number.isFinite(v) ? Math.min(1, Math.max(0, v)) : 1;
	},
	setVolume: (v: number) => set(KEYS.volume, String(Math.round(v * 100) / 100)),
	muted: () => get(KEYS.muted) === '1',
	setMuted: (m: boolean) => set(KEYS.muted, m ? '1' : '0'),
	quality(): QualityChoice {
		const q = get(KEYS.quality);
		return q === '1080p' || q === '720p' || q === '480p' ? q : 'auto';
	},
	setQuality: (q: QualityChoice) => set(KEYS.quality, q),
	subtitleSize(): SubtitleSize {
		const s = get(KEYS.subSize);
		return s === 's' || s === 'l' ? s : 'm';
	},
	setSubtitleSize: (s: SubtitleSize) => set(KEYS.subSize, s),
	stats: () => get(KEYS.stats) === '1',
	setStats: (on: boolean) => set(KEYS.stats, on ? '1' : '0')
};
