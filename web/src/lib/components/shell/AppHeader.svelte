<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { libraries, scans } from '$lib/api/stores.svelte';
	import { toasts } from '$lib/toast.svelte';
	import Logo from '../brand/Logo.svelte';
	import IconButton from '../IconButton.svelte';
	import SearchField from '../SearchField.svelte';
	import ThemeSwitcher from '../ThemeSwitcher.svelte';
	import UserMenu from './UserMenu.svelte';
	import { session } from '$lib/session.svelte';

	let scrolled = $state(false);
	let query = $state('');
	let timer: ReturnType<typeof setTimeout> | undefined;

	const onSearchPage = $derived(page.url.pathname === '/recherche');
	const running = $derived(Object.entries(scans.byLibrary).filter(([, s]) => s.running));

	// Mirror the URL on the search page (back/forward, links), without fighting the user's typing.
	$effect(() => {
		if (!onSearchPage) return;
		const q = page.url.searchParams.get('q') ?? '';
		if (q.trim() !== query.trim()) query = q;
	});

	function onScroll() {
		scrolled = window.scrollY > 8;
	}

	function navigate(replace: boolean) {
		const q = query.trim();
		goto(q ? `/recherche?q=${encodeURIComponent(q)}` : '/recherche', { keepFocus: true, noScroll: true, replaceState: replace });
	}

	function onInput() {
		clearTimeout(timer);
		if (onSearchPage) {
			// Already there: keep the URL in step (the page debounces the request itself).
			timer = setTimeout(() => navigate(true), 60);
		} else if (query.trim().length >= 2) {
			timer = setTimeout(() => navigate(false), 250);
		}
	}

	function submit(e: SubmitEvent) {
		e.preventDefault();
		clearTimeout(timer);
		navigate(onSearchPage);
	}

	function notifications() {
		if (!running.length) {
			toasts.show('Aucune notification', { detail: 'Les analyses de bibliothèques apparaîtront ici.' });
			return;
		}
		const names = running.map(([id]) => libraries.get(Number(id))?.name ?? `#${id}`).join(', ');
		toasts.show('Analyse en cours', { detail: names, action: session.isAdmin ? { label: 'Voir', run: () => goto('/reglages/bibliotheques') } : undefined });
	}

	$effect(onScroll);
</script>

<svelte:window onscroll={onScroll} />

<header class="header" class:scrolled>
	<a class="brand" href="/" aria-label="Aether — accueil"><Logo size={26} /></a>

	<form class="search" role="search" onsubmit={submit}>
		<SearchField bind:value={query} size="lg" shortcut placeholder="Rechercher un film, une série, un artiste…" oninput={onInput} />
	</form>

	<div class="end">
		<ThemeSwitcher />
		<IconButton
			icon="bell"
			badge={running.length > 0}
			aria-label={running.length ? `Notifications (${running.length} analyse${running.length > 1 ? 's' : ''} en cours)` : 'Notifications'}
			onclick={notifications}
		/>
		<span class="user-full"><UserMenu /></span>
		<span class="user-compact"><UserMenu compact /></span>
	</div>
</header>

<style>
	.header {
		position: sticky;
		top: 0;
		z-index: 20;
		display: flex;
		align-items: center;
		gap: var(--sp-4);
		height: var(--header-h);
		padding: 0 var(--gutter);
		background: var(--bg);
		transition: box-shadow var(--dur-base) var(--ease);
	}

	.header.scrolled {
		box-shadow: 0 1px 0 var(--line), 0 10px 24px -14px rgba(0, 0, 0, 0.45);
	}

	.brand {
		display: none;
		border-radius: var(--r-sm);
	}

	.search {
		flex: 0 1 520px;
		min-width: 0;
	}

	.search :global(.control) {
		border-radius: var(--r-md);
	}

	.end {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
		margin-left: auto;
	}

	.user-full {
		margin-left: var(--sp-2);
		min-width: 0;
		max-width: 260px;
	}

	.user-compact {
		display: none;
	}

	@media (max-width: 1023px) {
		.user-full {
			display: none;
		}

		.user-compact {
			display: block;
			margin-left: var(--sp-1);
		}
	}

	@media (max-width: 767px) {
		.header {
			height: 64px;
		}

		.brand {
			display: block;
		}

		.search {
			display: none;
		}
	}
</style>
