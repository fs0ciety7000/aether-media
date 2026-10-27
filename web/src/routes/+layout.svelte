<script lang="ts">
	import '@fontsource-variable/cormorant-garamond/wght.css';
	import '@fontsource-variable/cormorant-garamond/wght-italic.css';
	import '@fontsource-variable/instrument-sans/wght.css';
	import '$lib/styles/tokens.css';
	import '$lib/styles/base.css';

	import { onMount } from 'svelte';
	import AppHeader from '$lib/components/shell/AppHeader.svelte';
	import Sidebar from '$lib/components/shell/Sidebar.svelte';
	import TabBar from '$lib/components/shell/TabBar.svelte';
	import Toaster from '$lib/components/Toaster.svelte';

	let { children } = $props();

	const SIDEBAR_KEY = 'aether:sidebar';
	let collapsed = $state(readCollapsed());
	let version = $state<string>();

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

	// Server info is a nicety: the shell works without the API.
	onMount(() => {
		const ctrl = new AbortController();
		fetch('/api/v1/system/info', { signal: ctrl.signal, headers: { accept: 'application/json' } })
			.then((r) => (r.ok ? r.json() : null))
			.then((info) => {
				if (info && typeof info.version === 'string') version = info.version;
			})
			.catch(() => {});
		return () => ctrl.abort();
	});
</script>

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

<Toaster />

<style>
	.shell {
		--sidebar-w: 240px;
		display: grid;
		grid-template-columns: var(--sidebar-w) minmax(0, 1fr);
		min-height: 100vh;
		transition: grid-template-columns var(--dur-base) var(--ease);
	}

	.shell.collapsed {
		--sidebar-w: 76px;
	}

	.main {
		min-width: 0;
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
		font-size: var(--fs-small);
		font-weight: 500;
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
		}

		.tabs {
			display: block;
		}
	}
</style>
