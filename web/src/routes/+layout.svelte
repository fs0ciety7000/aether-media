<script lang="ts">
	import '@fontsource-variable/nunito/wght.css';
	import '$lib/styles/tokens.css';
	import '$lib/styles/base.css';

	import { onMount } from 'svelte';
	import AppHeader from '$lib/components/shell/AppHeader.svelte';
	import Sidebar from '$lib/components/shell/Sidebar.svelte';
	import TabBar from '$lib/components/shell/TabBar.svelte';
	import Toaster from '$lib/components/Toaster.svelte';
	import { libraries, scans, system, unmatched } from '$lib/api/stores.svelte';

	let { children } = $props();

	const SIDEBAR_KEY = 'aether:sidebar';
	let collapsed = $state(readCollapsed());
	const version = $derived(system.info?.version);

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

	// Shared data: everything degrades gracefully when the API is unreachable.
	onMount(() => {
		system.ensure();
		libraries.ensure();
		scans.ensure();
		unmatched.ensure();
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
