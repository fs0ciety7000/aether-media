// Placeholder catalogue for the M1 demo. Fictional titles; artwork is generated (see PosterArt / BackdropArt).

export type PosterLayout = 'sun' | 'horizon' | 'frame' | 'type' | 'column';
export type BackdropScene = 'dunes' | 'sea' | 'ridges' | 'room';

export interface DemoFilm {
	id: string;
	title: string;
	year: number;
	minutes: number;
	director: string;
	hue: number;
	/** Saturation of the artwork, kept low (≤ 24 %). */
	sat?: number;
	layout: PosterLayout;
	scene: BackdropScene;
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
		layout: 'sun',
		scene: 'dunes',
		tagline: 'Il reste toujours un peu de lumière.',
		synopsis:
			'Au cœur d’un été immobile, une photographe revient dans la maison de son enfance et découvre, dans les archives de son père, le portrait d’une femme que personne ne veut nommer.'
	},
	{
		id: 'maree-basse',
		title: 'Marée basse',
		year: 2021,
		minutes: 104,
		director: 'Jonas Lefèvre',
		hue: 200,
		layout: 'horizon',
		scene: 'sea',
		tagline: 'Ce que la mer rend, elle le reprend.',
		synopsis:
			'Sur une île de la Manche, un gardien de phare à la retraite et une biologiste venue du continent attendent la plus grande marée du siècle.'
	},
	{
		id: 'chambre-echo',
		title: 'La Chambre d’écho',
		year: 2017,
		minutes: 118,
		director: 'Inès Moreau',
		hue: 352,
		layout: 'frame',
		scene: 'room',
		tagline: 'Chaque mot finit par revenir.',
		synopsis:
			'Une ingénieure du son s’installe dans un hôtel désaffecté pour enregistrer son silence. Les murs, eux, n’ont rien oublié.'
	},
	{
		id: 'nord-magnetique',
		title: 'Nord magnétique',
		year: 2022,
		minutes: 141,
		director: 'Aksel Brandt',
		hue: 212,
		layout: 'type',
		scene: 'ridges',
		tagline: 'Plus loin que le dernier phare.',
		synopsis:
			'Trois géologues traversent le plateau du Finnmark pour relever une anomalie que leurs instruments refusent d’expliquer.'
	},
	{ id: 'dernier-ete', title: 'Le Dernier Été', year: 2016, minutes: 97, director: 'Camille Roche', hue: 24, layout: 'column', scene: 'room', progress: 0.42 },
	{ id: 'ombres-portees', title: 'Ombres portées', year: 2020, minutes: 110, director: 'Paul Ardent', hue: 18, sat: 12, layout: 'sun', scene: 'dunes' },
	{ id: 'atlas', title: 'Atlas', year: 2023, minutes: 156, director: 'Nora Lindqvist', hue: 150, layout: 'type', scene: 'ridges' },
	{ id: 'fugitifs', title: 'Les Fugitifs du matin', year: 2018, minutes: 101, director: 'Mathis Duret', hue: 42, layout: 'horizon', scene: 'sea' },
	{ id: 'dune-de-sel', title: 'Dune de sel', year: 2024, minutes: 124, director: 'Leïla Haddad', hue: 28, layout: 'frame', scene: 'dunes', progress: 0.78 },
	{ id: 'nocturne', title: 'Nocturne', year: 2015, minutes: 89, director: 'Élise Marchal', hue: 232, sat: 14, layout: 'column', scene: 'room' },
	{ id: 'jardin-hiver', title: 'Le Jardin d’hiver', year: 2022, minutes: 115, director: 'Victor Anselme', hue: 95, sat: 14, layout: 'sun', scene: 'ridges' },
	{ id: 'vertige', title: 'Vertige des sommets', year: 2019, minutes: 128, director: 'Tomas Reiner', hue: 190, layout: 'type', scene: 'ridges' },
	{ id: 'ligne-horizon', title: 'La Ligne d’horizon', year: 2025, minutes: 134, director: 'Sacha Morel', hue: 12, layout: 'horizon', scene: 'sea' }
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
	scene: BackdropScene;
}

export const continueWatching: (DemoEpisode | (DemoFilm & { progress: number }))[] = [
	{ id: 'veilleurs-s2e4', show: 'Les Veilleurs', season: 2, episode: 4, title: 'Le Phare de Kéréon', minutes: 52, progress: 0.35, hue: 205, scene: 'sea' },
	{ ...films[4], progress: 0.42 } as DemoFilm & { progress: number },
	{ id: 'hautes-terres-s1e7', show: 'Hautes Terres', season: 1, episode: 7, title: 'Ce qui reste du feu', minutes: 47, progress: 0.68, hue: 30, scene: 'ridges' },
	{ ...films[8], progress: 0.78 } as DemoFilm & { progress: number },
	{ id: 'archipel-s1e2', show: 'Archipel', season: 1, episode: 2, title: 'La Traversée', minutes: 58, progress: 0.12, hue: 180, scene: 'dunes' },
	{ id: 'brigade-s3e1', show: 'La Brigade du fleuve', season: 3, episode: 1, title: 'Crue', minutes: 44, progress: 0.9, hue: 350, scene: 'room' }
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
	{ id: 'deserts', title: 'Lumières du désert', count: 6, filmIds: ['heures-claires', 'dune-de-sel', 'ombres-portees'] },
	{ id: 'premiers-films', title: 'Premiers films', count: 14, filmIds: ['atlas', 'nocturne', 'maree-basse'] }
];

export const filmById = (id: string) => films.find((f) => f.id === id)!;

export function isEpisode(item: DemoEpisode | DemoFilm): item is DemoEpisode {
	return 'show' in item;
}

/** Dominant colour of an artwork — what a blurhash would give us. */
export function dominant(hue: number, sat = 18) {
	return `hsl(${hue} ${sat}% 22%)`;
}
