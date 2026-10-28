// PWA (T-082): install prompt state. Chromium fires `beforeinstallprompt` early, often before the
// layout mounts, so the event is captured as soon as this module loads. iOS has no such event:
// installation goes through Safari's Share sheet, which we only mention (once, dismissible).

interface BeforeInstallPromptEvent extends Event {
	prompt(): Promise<void>;
	userChoice: Promise<{ outcome: 'accepted' | 'dismissed' }>;
}

const DISMISS_KEY = 'aether:install-dismissed';

function readDismissed(): boolean {
	try {
		const v = localStorage.getItem(DISMISS_KEY);
		// Asked again after 60 days.
		return !!v && Date.now() - Number(v) < 60 * 86_400_000;
	} catch {
		return false;
	}
}

class InstallState {
	/** Chromium: the deferred prompt, when the browser considers the app installable. */
	deferred = $state<BeforeInstallPromptEvent | null>(null);
	/** Already running as an installed app. */
	standalone = $state(false);
	/** iPhone / iPad in Safari, not installed. */
	ios = $state(false);
	dismissed = $state(false);
	online = $state(true);

	constructor() {
		if (typeof window === 'undefined') return;
		this.dismissed = readDismissed();
		this.online = navigator.onLine;
		const nav = navigator as Navigator & { standalone?: boolean };
		this.standalone = window.matchMedia('(display-mode: standalone)').matches || nav.standalone === true;
		const ua = navigator.userAgent;
		const iOS = /iPhone|iPad|iPod/.test(ua) || (ua.includes('Macintosh') && navigator.maxTouchPoints > 1);
		this.ios = iOS && /Safari/.test(ua) && !/CriOS|FxiOS|EdgiOS/.test(ua);
		window.addEventListener('beforeinstallprompt', (e) => {
			e.preventDefault();
			this.deferred = e as BeforeInstallPromptEvent;
		});
		window.addEventListener('appinstalled', () => {
			this.deferred = null;
			this.standalone = true;
		});
		window.addEventListener('online', () => (this.online = true));
		window.addEventListener('offline', () => (this.online = false));
	}

	/** Something to offer: the browser prompt, or the iOS hint. */
	get canOffer(): boolean {
		return !this.standalone && !this.dismissed && (!!this.deferred || this.ios);
	}

	async install(): Promise<boolean> {
		const d = this.deferred;
		if (!d) return false;
		this.deferred = null;
		await d.prompt();
		const choice = await d.userChoice;
		if (choice.outcome === 'dismissed') this.dismiss();
		return choice.outcome === 'accepted';
	}

	dismiss() {
		this.dismissed = true;
		try {
			localStorage.setItem(DISMISS_KEY, String(Date.now()));
		} catch {
			/* not persisted */
		}
	}
}

export const pwa = new InstallState();
