// Current user (GET /me), loaded once at startup, and the first-run state (system/info.setupRequired).
import { api, ApiError, normalizeUser } from '$lib/api/client';
import type { MePatch, User } from '$lib/api/types';

export type SessionStatus = 'unknown' | 'authenticated' | 'anonymous' | 'setup' | 'unreachable';

/** Routes reachable without a session (rendered without the app shell). */
export const PUBLIC_ROUTES = ['/connexion', '/bienvenue'];

export function isPublicPath(pathname: string): boolean {
	return PUBLIC_ROUTES.some((p) => pathname === p || pathname.startsWith(p + '/'));
}

/** Login URL that brings the user back to `path` afterwards. */
export function loginHref(path: string): string {
	return !path || path === '/' || isPublicPath(path.split('?')[0]) ? '/connexion' : `/connexion?retour=${encodeURIComponent(path)}`;
}

/** Only same-app paths are honoured as a return target (no open redirect). */
export function safeReturn(raw: string | null | undefined): string {
	if (!raw || !raw.startsWith('/') || raw.startsWith('//') || raw.startsWith('/\\')) return '/';
	return isPublicPath(raw.split('?')[0]) ? '/' : raw;
}

/** Short device label sent with login / setup ("Firefox · Linux"). */
export function deviceLabel(): string {
	if (typeof navigator === 'undefined') return 'Navigateur';
	const ua = navigator.userAgent;
	const browser = /Edg\//.test(ua)
		? 'Edge'
		: /OPR\//.test(ua)
			? 'Opera'
			: /Firefox\//.test(ua)
				? 'Firefox'
				: /Chrome\//.test(ua)
					? 'Chrome'
					: /Safari\//.test(ua)
						? 'Safari'
						: 'Navigateur';
	const os = /iPhone|iPad/.test(ua)
		? 'iOS'
		: /Android/.test(ua)
			? 'Android'
			: /Mac OS X/.test(ua)
				? 'macOS'
				: /Windows/.test(ua)
					? 'Windows'
					: /Linux/.test(ua)
						? 'Linux'
						: '';
	return os ? `${browser} · ${os}` : browser;
}

class SessionStore {
	user = $state<User | null>(null);
	status = $state<SessionStatus>('unknown');
	isAdmin = $derived(!!this.user?.isAdmin);
	/** M7: an administrator left the first-run assistant before its end (it resumes). */
	wizardPending = $state(false);
	#pending: Promise<void> | null = null;

	/** Loads once; later calls return the same promise. */
	init(): Promise<void> {
		return (this.#pending ??= this.#load());
	}

	/** Re-reads system/info and /me (after a 401, for instance). */
	reload(): Promise<void> {
		this.#pending = this.#load();
		return this.#pending;
	}

	async #load() {
		let wizard = false;
		try {
			const info = await api.systemInfo();
			wizard = !!info?.setupWizardPending;
			if (info?.setupRequired) {
				this.user = null;
				this.status = 'setup';
				return;
			}
			if (info?.authenticated === false) {
				// No session: skip /me (a 401 that would only clutter the console).
				this.user = null;
				this.status = 'anonymous';
				return;
			}
		} catch {
			// system/info is public: failing means the server is down. /me will tell.
		}
		try {
			this.user = await api.me();
			this.wizardPending = wizard && this.user.isAdmin;
			this.status = 'authenticated';
		} catch (err) {
			if (err instanceof ApiError && err.status === 401) {
				this.user = null;
				this.status = 'anonymous';
			} else if (err instanceof ApiError && err.status === 404) {
				// Server older than M6: no accounts, everything is allowed.
				this.user = normalizeUser({ id: 0, name: 'Administrateur', isAdmin: true, allLibraries: true, libraryIds: [], blockUnrated: false, prefs: {}, createdAt: '' });
				this.status = 'authenticated';
			} else {
				this.status = 'unreachable';
			}
		}
	}

	/** Called on any 401 from the API. */
	expire() {
		if (this.status === 'authenticated') {
			this.user = null;
			this.status = 'anonymous';
		}
	}

	signedIn(user: User) {
		this.user = user;
		this.status = 'authenticated';
		this.#pending = Promise.resolve();
	}

	async login(name: string, password: string) {
		const r = await api.login({ name, password, device: deviceLabel() });
		if (r.user.isAdmin) {
			// The assistant resumes for an administrator who left it before its end.
			try {
				this.wizardPending = !!(await api.systemInfo())?.setupWizardPending;
			} catch {
				this.wizardPending = false;
			}
		}
		this.signedIn(r.user);
	}

	async setup(name: string, password: string) {
		const r = await api.setup({ name, password, device: deviceLabel() });
		this.wizardPending = true;
		this.signedIn(r.user);
	}

	async logout() {
		try {
			await api.logout();
		} catch {
			/* the cookie is gone either way */
		}
		this.user = null;
		this.status = 'anonymous';
		this.wizardPending = false;
		this.#pending = Promise.resolve();
	}

	async update(patch: MePatch) {
		const u = await api.patchMe(patch);
		this.user = u;
		return u;
	}
}

export const session = new SessionStore();

export const ROLE_LABEL = (u: Pick<User, 'isAdmin'> | null | undefined) => (u?.isAdmin ? 'Administrateur' : 'Membre');
