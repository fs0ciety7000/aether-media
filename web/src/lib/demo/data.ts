// Placeholder catalogue for the demo. Fictional titles; artwork is generated (see PosterArt / BackdropArt).

export type PosterLayout = 'sun' | 'horizon' | 'frame' | 'type' | 'column';
export type BackdropScene = 'dunes' | 'sea' | 'ridges' | 'room' | 'city' | 'forest' | 'aurora' | 'planet';

export interface DemoFilm {
	id: string;
	title: string;
	year: number;
	minutes: number;
	director: string;
	/** Key-light hue of the artwork (glow, horizon). */
	hue: number;
	/** Hue of the night side of the artwork (sky, shadows) — navy/violet range. */
	hue2?: number;
	/** Saturation of the artwork (0–70). */
	sat?: number;
	layout: PosterLayout;
	scene: BackdropScene;
	genres: string[];
	tagline?: string;
	synopsis?: string;
	progress?: number;
}

export const films: DemoFilm[] = [
	{
		id: 'heures-claires',
		title: 'Les Heures claires',
		year: 2019,
		minutes: 132,
		director: 'Hélène Varga',
		hue: 32,
		hue2: 252,
		layout: 'sun',
		scene: 'dunes',
		genres: ['Drame', 'Aventure'],
		tagline: 'Il reste toujours un peu de lumière.',
		synopsis:
			'Au cœur d’un été immobile, une photographe revient dans la maison de son enfance et découvre le portrait d’une femme que personne ne veut nommer.'
	},
	{
		id: 'nord-magnetique',
		title: 'Nord magnétique',
		year: 2022,
		minutes: 141,
		director: 'Aksel Brandt',
		hue: 168,
		hue2: 232,
		layout: 'type',
		scene: 'aurora',
		genres: ['Aventure', 'Science-fiction'],
		tagline: 'Plus loin que le dernier phare.',
		synopsis:
			'Trois géologues traversent le plateau du Finnmark pour relever une anomalie que leurs instruments refusent d’expliquer.'
	},
	{
		id: 'maree-basse',
		title: 'Marée basse',
		year: 2021,
		minutes: 104,
		director: 'Jonas Lefèvre',
		hue: 188,
		hue2: 226,
		layout: 'horizon',
		scene: 'sea',
		genres: ['Drame', 'Mystère'],
		tagline: 'Ce que la mer rend, elle le reprend.',
		synopsis:
			'Sur une île de la Manche, un gardien de phare à la retraite et une biologiste venue du continent attendent la plus grande marée du siècle.'
	},
	{
		id: 'nocturne',
		title: 'Nocturne',
		year: 2024,
		minutes: 109,
		director: 'Élise Marchal',
		hue: 282,
		hue2: 236,
		layout: 'column',
		scene: 'city',
		genres: ['Thriller', 'Néo-noir'],
		tagline: 'La ville ne dort jamais vraiment.',
		synopsis:
			'Une chauffeuse de taxi de nuit devient, malgré elle, l’unique témoin d’un échange qui n’aurait jamais dû avoir lieu.'
	},
	{ id: 'chambre-echo', title: 'La Chambre d’écho', year: 2017, minutes: 118, director: 'Inès Moreau', hue: 345, hue2: 262, layout: 'frame', scene: 'room', genres: ['Thriller'] },
	{ id: 'dernier-ete', title: 'Le Dernier Été', year: 2016, minutes: 97, director: 'Camille Roche', hue: 26, hue2: 244, layout: 'column', scene: 'room', genres: ['Drame'], progress: 0.42 },
	{ id: 'ombres-portees', title: 'Ombres portées', year: 2020, minutes: 110, director: 'Paul Ardent', hue: 36, hue2: 228, layout: 'sun', scene: 'city', genres: ['Policier'] },
	{ id: 'atlas', title: 'Atlas', year: 2023, minutes: 156, director: 'Nora Lindqvist', hue: 198, hue2: 266, layout: 'type', scene: 'planet', genres: ['Science-fiction'] },
	{ id: 'fugitifs', title: 'Les Fugitifs du matin', year: 2018, minutes: 101, director: 'Mathis Duret', hue: 40, hue2: 214, layout: 'horizon', scene: 'forest', genres: ['Aventure'] },
	{ id: 'dune-de-sel', title: 'Dune de sel', year: 2024, minutes: 124, director: 'Leïla Haddad', hue: 18, hue2: 278, layout: 'frame', scene: 'dunes', genres: ['Aventure'], progress: 0.78 },
	{ id: 'jardin-hiver', title: 'Le Jardin d’hiver', year: 2022, minutes: 115, director: 'Victor Anselme', hue: 158, hue2: 222, layout: 'sun', scene: 'forest', genres: ['Fantastique'] },
	{ id: 'vertige', title: 'Vertige des sommets', year: 2019, minutes: 128, director: 'Tomas Reiner', hue: 212, hue2: 252, layout: 'type', scene: 'ridges', genres: ['Documentaire'] },
	{ id: 'ligne-horizon', title: 'La Ligne d’horizon', year: 2025, minutes: 134, director: 'Sacha Morel', hue: 22, hue2: 240, layout: 'horizon', scene: 'sea', genres: ['Romance'] }
];

export interface DemoEpisode {
	id: string;
	show: string;
	season: number;
	episode: number;
	title: string;
	minutes: number;
	progress: number;
	hue: number;
	hue2?: number;
	scene: BackdropScene;
}

export type ContinueItem = DemoEpisode | (DemoFilm & { progress: number });

export const continueWatching: ContinueItem[] = [
	{ id: 'veilleurs-s2e4', show: 'Les Veilleurs', season: 2, episode: 4, title: 'Le Phare de Kéréon', minutes: 90, progress: 0.64, hue: 278, hue2: 232, scene: 'city' },
	{ id: 'archipel-s1e2', show: 'Archipel', season: 1, episode: 2, title: 'La Traversée', minutes: 29, progress: 0.86, hue: 196, hue2: 238, scene: 'planet' },
	{ id: 'hautes-terres-s1e7', show: 'Hautes Terres', season: 1, episode: 7, title: 'Ce qui reste du feu', minutes: 60, progress: 0.38, hue: 120, hue2: 210, scene: 'forest' },
	{ ...films[5], progress: 0.42 } as DemoFilm & { progress: number },
	{ id: 'brigade-s3e1', show: 'La Brigade du fleuve', season: 3, episode: 1, title: 'Crue', minutes: 18, progress: 0.72, hue: 330, hue2: 258, scene: 'aurora' },
	{ ...films[9], progress: 0.27 } as DemoFilm & { progress: number }
];

export interface DemoGenre {
	id: string;
	label: string;
	hue: number;
	hue2: number;
	scene: BackdropScene;
}

export const genres: DemoGenre[] = [
	{ id: 'action', label: 'Action', hue: 20, hue2: 236, scene: 'city' },
	{ id: 'comedie', label: 'Comédie', hue: 44, hue2: 212, scene: 'dunes' },
	{ id: 'fantastique', label: 'Fantastique', hue: 286, hue2: 236, scene: 'forest' },
	{ id: 'thriller', label: 'Thriller', hue: 205, hue2: 228, scene: 'ridges' },
	{ id: 'romance', label: 'Romance', hue: 345, hue2: 262, scene: 'sea' },
	{ id: 'horreur', label: 'Horreur', hue: 140, hue2: 220, scene: 'room' },
	{ id: 'animation', label: 'Animation', hue: 182, hue2: 270, scene: 'planet' },
	{ id: 'drame', label: 'Drame', hue: 28, hue2: 248, scene: 'aurora' }
];

export interface DemoCollection {
	id: string;
	title: string;
	count: number;
	filmIds: string[];
}

export const collections: DemoCollection[] = [
	{ id: 'cinema-hiver', title: 'Cinéma d’hiver', count: 8, filmIds: ['nord-magnetique', 'jardin-hiver', 'vertige'] },
	{ id: 'bord-de-mer', title: 'Bords de mer', count: 11, filmIds: ['maree-basse', 'fugitifs', 'ligne-horizon'] },
	{ id: 'huis-clos', title: 'Huis clos', count: 7, filmIds: ['chambre-echo', 'nocturne', 'dernier-ete'] },
	{ id: 'deserts', title: 'Lumières du désert', count: 6, filmIds: ['heures-claires', 'dune-de-sel', 'ombres-portees'] }
];

export const currentUser = { name: 'Camille Laurent', email: 'camille@aether.local', hue: 186 };

export const filmById = (id: string) => films.find((f) => f.id === id)!;

export function isEpisode(item: DemoEpisode | DemoFilm): item is DemoEpisode {
	return 'show' in item;
}

/** Dominant colour of an artwork — what a blurhash would give us. */
export function dominant(hue: number, hue2 = 230) {
	return `hsl(${hue2} 42% 12%)`;
}
