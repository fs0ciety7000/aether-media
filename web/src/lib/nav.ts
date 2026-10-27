import type { IconName } from '$lib/components/Icon.svelte';

export interface NavItem {
	href: string;
	label: string;
	icon: IconName;
}

export const home: NavItem = { href: '/', label: 'Accueil', icon: 'home' };

/** Sidebar, first group: discovery. */
export const discover: NavItem[] = [
	home,
	{ href: '/explorer', label: 'Explorer', icon: 'compass' },
	{ href: '/favoris', label: 'Favoris', icon: 'heart' },
	{ href: '/genres', label: 'Genres', icon: 'genres' }
];

/** Sidebar, second group: the user's library. */
export const library: NavItem[] = [
	{ href: '/collection', label: 'Ma collection', icon: 'layers' },
	{ href: '/recents', label: 'Récemment ajoutés', icon: 'monitor' },
	{ href: '/reprendre', label: 'Reprendre', icon: 'clock' },
	{ href: '/collections', label: 'Collections', icon: 'folder' }
];

export const settings: NavItem = { href: '/reglages', label: 'Réglages', icon: 'settings' };
export const search: NavItem = { href: '/recherche', label: 'Recherche', icon: 'search' };
export const profile: NavItem = { href: '/profil', label: 'Profil', icon: 'user' };

/** Mobile tab bar: five destinations at most. */
export const tabs: NavItem[] = [home, discover[1], search, discover[2], profile];

export function isActive(pathname: string, href: string) {
	return href === '/' ? pathname === '/' : pathname === href || pathname.startsWith(href + '/');
}
