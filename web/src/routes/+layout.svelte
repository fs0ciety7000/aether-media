<script lang="ts">
	import '@fontsource-variable/nunito/wght.css';
	import '$lib/styles/tokens.css';
	import '$lib/styles/base.css';

	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { setAuthHandlers } from '$lib/api/client';
	import Button from '$lib/components/Button.svelte';
	import Logo from '$lib/components/brand/Logo.svelte';
	import { isPublicPath, loginHref, session } from '$lib/session.svelte';
	import { toasts } from '$lib/toast.svelte';
	import AppHeader from '$lib/components/shell/AppHeader.svelte';
	import Sidebar from '$lib/components/shell/Sidebar.svelte';
	import TabBar from '$lib/components/shell/TabBar.svelte';
	import Toaster from '$lib/components/Toaster.svelte';
	import { libraries, scans, system, unmatched } from '$lib/api/stores.svelte';

	let { children } = $props();

	const SIDEBAR_KEY = 'aether:sidebar';
	let collapsed = $state(readCollapsed());
	const version = $derived(system.info?.version);
	const publicRoute = $derived(isPublicPath(page.url.pathname));
	/** Full-screen routes (the player, login, first run) render without the app shell. */
	const bare = $derived((page.route.id?.startsWith('/lecture') ?? false) || publicRoute);
	const ready = $derived(publicRoute || session.status === 'authenticated');

	function readCollapsed() {
		try {
			return localStorage.getItem(SIDEBAR_KEY) === 'collapsed';
		} catch {
			return false;
		}
	}

	function toggleSidebar() {
		collapsed = !collapsed;
		try {
			localStorage.setItem(SIDEBAR_KEY, collapsed ? 'collapsed' : 'expanded');
		} catch {
			/* not persisted */
		}
	}

	const here = () => page.url.pathname + page.url.search;
	let lastForbidden = 0;

	onMount(() => {
		system.ensure();
		setAuthHandlers({
			onUnauthenticated: () => {
				session.expire();
				if (!isPublicPath(page.url.pathname)) goto(loginHref(here()), { replaceState: true });
			},
			onForbidden: () => {
				// One toast per burst (several requests can be refused at once).
				if (Date.now() - lastForbidden < 3000) return;
				lastForbidden = Date.now();
				toasts.show('Action réservée aux administrateurs');
			}
		});
		session.init();
	});

	// Gate every private route on the session; send first runs to the assistant.
	$effect(() => {
		const status = session.status;
		if (publicRoute) return;
		if (status === 'setup') goto('/bienvenue', { replaceState: true });
		else if (status === 'anonymous') goto(loginHref(here()), { replaceState: true });
	});

	// Shared data, once signed in (reloaded when another account signs in).
	let dataFor: number | null = null;
	$effect(() => {
		const user = session.user;
		if (session.status !== 'authenticated' || !user) return;
		if (dataFor === user.id) return;
		const first = dataFor === null;
		dataFor = user.id;
		libraries.ensure();
		scans.ensure();
		if (!first) libraries.refresh();
		if (user.isAdmin) {
			unmatched.ensure();
			if (!first) unmatched.refresh();
		} else unmatched.count = 0;
	});
</script>

{#if !ready}
	<div class="splash" aria-busy={session.status === 'unknown'}>
		<Logo size={44} wordmark={false} />
		{#if session.status === 'unreachable'}
			<div class="splash-text" role="alert">
				<p class="splash-title">Le serveur est injoignable</p>
				<p>Vérifiez qu’Aether est démarré, puis réessayez.</p>
				<Button icon="refresh" onclick={() => session.reload()}>Réessayer</Button>
			</div>
		{:else}
			<span class="visually-hidden">Chargement…</span>
		{/if}
	</div>
{:else if bare}
	{@render children()}
{:else}
	<a class="skip" href="#contenu">Aller au contenu</a>

	<div class="shell" class:collapsed>
		<div class="side">
			<Sidebar {collapsed} ontoggle={toggleSidebar} {version} />
		</div>

		<div class="main">
			<AppHeader />
			<main id="contenu" tabindex="-1">
				{@render children()}
			</main>
		</div>
	</div>

	<div class="tabs">
		<TabBar />
	</div>
{/if}

<Toaster />

<style>
	.splash {
		display: grid;
		place-content: center;
		justify-items: center;
		gap: var(--sp-5);
		min-height: 100vh;
		min-height: 100dvh;
		padding: var(--sp-5);
		background: var(--bg);
		text-align: center;
	}

	.splash :global(.logo) {
		animation: breathe 1.6s var(--ease) infinite alternate;
	}

	.splash-text {
		display: grid;
		justify-items: center;
		gap: var(--sp-3);
		color: var(--text-muted);
	}

	.splash-title {
		color: var(--text);
		font-size: var(--fs-h2);
		font-weight: var(--fw-bold);
	}

	@keyframes breathe {
		from {
			opacity: 0.45;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.splash :global(.logo) {
			animation: none;
		}
	}

	.shell {
		--sidebar-w: 248px;
		display: grid;
		grid-template-columns: var(--sidebar-w) minmax(0, 1fr);
		min-height: 100vh;
		transition: grid-template-columns var(--dur-base) var(--ease);
	}

	.shell.collapsed {
		--sidebar-w: 76px;
	}

	.side {
		box-shadow: inset -1px 0 0 var(--line);
	}

	.main {
		min-width: 0;
	}

	main {
		display: block;
	}

	main:focus {
		outline: none;
	}

	.tabs {
		display: none;
	}

	.skip {
		position: fixed;
		z-index: 100;
		left: var(--sp-4);
		top: var(--sp-4);
		padding: var(--sp-2) var(--sp-4);
		border-radius: var(--r-md);
		background: var(--accent);
		color: var(--accent-contrast);
		font-size: var(--fs-body);
		font-weight: var(--fw-bold);
		transform: translateY(-200%);
	}

	.skip:focus-visible {
		transform: none;
	}

	@media (max-width: 767px) {
		.shell {
			display: block;
		}

		.side {
			display: none;
		}

		.main {
			padding-bottom: calc(var(--tabbar-h) + env(safe-area-inset-bottom, 0px));
			overflow-x: clip;
		}

		.tabs {
			display: block;
		}
	}
</style>
