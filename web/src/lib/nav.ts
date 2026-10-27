import type { IconName } from '$lib/components/Icon.svelte';

export interface NavItem {
	href: string;
	label: string;
	icon: IconName;
}

export const home: NavItem = { href: '/', label: 'Accueil', icon: 'home' };

export const libraries: NavItem[] = [
	{ href: '/films', label: 'Films', icon: 'film' },
	{ href: '/series', label: 'Séries', icon: 'tv' },
	{ href: '/musique', label: 'Musique', icon: 'music' },
	{ href: '/collections', label: 'Collections', icon: 'collection' }
];

export const search: NavItem = { href: '/recherche', label: 'Recherche', icon: 'search' };

export const account: NavItem[] = [
	{ href: '/reglages', label: 'Réglages', icon: 'settings' },
	{ href: '/profil', label: 'Profil', icon: 'user' }
];

/** Mobile tab bar: five destinations at most. */
export const tabs: NavItem[] = [home, libraries[0], libraries[1], libraries[2], search];

export function isActive(pathname: string, href: string) {
	return href === '/' ? pathname === '/' : pathname === href || pathname.startsWith(href + '/');
}
