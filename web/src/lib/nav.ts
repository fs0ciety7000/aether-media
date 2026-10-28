import type { IconName } from '$lib/components/Icon.svelte';

export interface NavItem {
	href: string;
	label: string;
	icon: IconName;
	/** Route prefix that marks the entry active (defaults to href). */
	match?: string;
}

export const home: NavItem = { href: '/', label: 'Accueil', icon: 'home' };
export const search: NavItem = { href: '/recherche', label: 'Recherche', icon: 'search' };
export const libraries: NavItem = { href: '/bibliotheque', label: 'Bibliothèques', icon: 'layers' };
export const collections: NavItem = { href: '/collections', label: 'Collections', icon: 'collection' };
export const watchlist: NavItem = { href: '/ma-liste', label: 'Ma liste', icon: 'bookmark' };
export const identify: NavItem = { href: '/a-identifier', label: 'À identifier', icon: 'wand' };
export const settings: NavItem = { href: '/reglages/bibliotheques', label: 'Réglages', icon: 'settings', match: '/reglages' };
export const profile: NavItem = { href: '/profil', label: 'Profil', icon: 'user' };

/** Sidebar, first group. */
export const discover: NavItem[] = [home, search, watchlist];

/** Mobile tab bar: five destinations at most (members get Collections instead of Réglages). */
export function tabsFor(isAdmin: boolean): NavItem[] {
	return [home, libraries, search, isAdmin ? settings : collections, profile];
}

/** Settings sub-navigation (admins). */
export const settingsNav: NavItem[] = [
	{ href: '/reglages/bibliotheques', label: 'Bibliothèques', icon: 'layers' },
	{ href: '/reglages/utilisateurs', label: 'Utilisateurs', icon: 'users' },
	{ href: '/reglages/synchronisation', label: 'Synchronisation', icon: 'refresh' }
];

/** Profile sub-navigation (everyone). */
export const profileNav: NavItem[] = [
	{ href: '/profil', label: 'Compte', icon: 'user' },
	{ href: '/profil/synchronisation', label: 'Synchronisation', icon: 'refresh' }
];

export function isActive(pathname: string, href: string) {
	return href === '/' ? pathname === '/' : pathname === href || pathname.startsWith(href + '/');
}
