// Theme & grain preferences. The initial values are applied before first paint
// by the inline script in app.html; this store keeps them in sync afterwards.

export type ThemeId = 'salle-obscure' | 'velours' | 'ardoise' | 'noyer' | 'lin';
export type ThemePref = ThemeId | 'system';

export interface ThemeInfo {
	id: ThemeId;
	label: string;
	mood: string;
	/** Preview colours (bg, surface, text, accent) — mirrors tokens.css. */
	swatch: { bg: string; surface: string; text: string; accent: string };
}

export const THEMES: ThemeInfo[] = [
	{ id: 'salle-obscure', label: 'Salle obscure', mood: 'Sombre et chaud', swatch: { bg: '#121010', surface: '#1A1716', text: '#EDE6DA', accent: '#B89A6A' } },
	{ id: 'velours', label: 'Velours', mood: 'Rideau bordeaux', swatch: { bg: '#140F10', surface: '#1D1617', text: '#EFE5DF', accent: '#A8786A' } },
	{ id: 'ardoise', label: 'Ardoise', mood: 'Minéral, nordique', swatch: { bg: '#101214', surface: '#181B1E', text: '#E4E6E8', accent: '#8FA3AE' } },
	{ id: 'noyer', label: 'Noyer', mood: 'Bibliothèque', swatch: { bg: '#13110E', surface: '#1C1915', text: '#ECE4D4', accent: '#A8875A' } },
	{ id: 'lin', label: 'Lin', mood: 'Matinée, papier', swatch: { bg: '#F3EFE8', surface: '#FBF9F5', text: '#2A2622', accent: '#8A6E45' } }
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
	pref = $state<ThemePref>('salle-obscure');
	grain = $state(true);
	#systemLight = $state(lightQuery?.matches ?? false);

	/** The theme actually displayed once "system" is resolved. */
	resolved = $derived<ThemeId>(this.pref === 'system' ? (this.#systemLight ? 'lin' : 'salle-obscure') : this.pref);

	constructor() {
		const stored = read(THEME_KEY) as ThemePref | null;
		if (stored && PREFS.includes(stored)) this.pref = stored;
		this.grain = read(GRAIN_KEY) !== 'off';
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
