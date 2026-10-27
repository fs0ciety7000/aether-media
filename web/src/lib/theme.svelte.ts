// Theme & grain preferences. The initial values are applied before first paint
// by the inline script in app.html; this store keeps them in sync afterwards.

export type ThemeId = 'nuit' | 'salle-obscure' | 'velours' | 'ardoise' | 'noyer' | 'lin';
export type ThemePref = ThemeId | 'system';

export interface ThemeInfo {
	id: ThemeId;
	label: string;
	mood: string;
	/** Preview colours (bg, surface, text, accent) — mirrors tokens.css. */
	swatch: { bg: string; surface: string; text: string; accent: string };
}

export const THEMES: ThemeInfo[] = [
	{ id: 'nuit', label: 'Nuit', mood: 'Bleu nuit, sarcelle', swatch: { bg: '#04121D', surface: '#0D2F3D', text: '#F6FFFF', accent: '#5EC4C0' } },
	{ id: 'salle-obscure', label: 'Salle obscure', mood: 'Sombre, ambre', swatch: { bg: '#121010', surface: '#2E2620', text: '#EDE6DA', accent: '#C9A673' } },
	{ id: 'velours', label: 'Velours', mood: 'Rideau bordeaux', swatch: { bg: '#140F10', surface: '#362123', text: '#EFE5DF', accent: '#CF8E80' } },
	{ id: 'ardoise', label: 'Ardoise', mood: 'Minéral, nordique', swatch: { bg: '#101214', surface: '#222F37', text: '#E4E6E8', accent: '#93B6C7' } },
	{ id: 'noyer', label: 'Noyer', mood: 'Bibliothèque', swatch: { bg: '#13110E', surface: '#30281C', text: '#ECE4D4', accent: '#C49C64' } },
	{ id: 'lin', label: 'Lin', mood: 'Clair, papier', swatch: { bg: '#F3EFE8', surface: '#DCEBE7', text: '#22262A', accent: '#2C7B77' } }
];

const THEME_KEY = 'aether:theme';
const GRAIN_KEY = 'aether:grain';
const PREFS: ThemePref[] = [...THEMES.map((t) => t.id), 'system'];

function read(key: string): string | null {
	try {
		return localStorage.getItem(key);
	} catch {
		return null;
	}
}

function write(key: string, value: string) {
	try {
		localStorage.setItem(key, value);
	} catch {
		/* private mode, blocked storage: the choice simply won't persist */
	}
}

const lightQuery = typeof matchMedia === 'function' ? matchMedia('(prefers-color-scheme: light)') : null;

class ThemeStore {
	pref = $state<ThemePref>('nuit');
	grain = $state(false);
	#systemLight = $state(lightQuery?.matches ?? false);

	/** The theme actually displayed once "system" is resolved. */
	resolved = $derived<ThemeId>(this.pref === 'system' ? (this.#systemLight ? 'lin' : 'nuit') : this.pref);

	constructor() {
		const stored = read(THEME_KEY) as ThemePref | null;
		if (stored && PREFS.includes(stored)) this.pref = stored;
		// Grain is opt-in since the Nuit direction.
		this.grain = read(GRAIN_KEY) === 'on';
		lightQuery?.addEventListener('change', (e) => {
			this.#systemLight = e.matches;
			this.#syncMeta();
		});
	}

	set(pref: ThemePref) {
		this.pref = pref;
		write(THEME_KEY, pref);
		this.#apply();
	}

	setGrain(on: boolean) {
		this.grain = on;
		write(GRAIN_KEY, on ? 'on' : 'off');
		this.#apply();
	}

	#apply() {
		if (typeof document === 'undefined') return;
		const root = document.documentElement;
		root.dataset.theme = this.pref;
		root.dataset.grain = this.grain ? 'on' : 'off';
		this.#syncMeta();
	}

	#syncMeta() {
		const meta = document.querySelector<HTMLMetaElement>('meta[name="theme-color"]');
		const info = THEMES.find((t) => t.id === this.resolved);
		if (meta && info) meta.content = info.swatch.bg;
	}
}

export const theme = new ThemeStore();
