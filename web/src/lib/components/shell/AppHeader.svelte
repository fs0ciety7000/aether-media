<script lang="ts">
	import { goto } from '$app/navigation';
	import { shell } from '$lib/shell.svelte';
	import Icon from '../Icon.svelte';
	import SearchField from '../SearchField.svelte';
	import ThemeSwitcher from '../ThemeSwitcher.svelte';

	let scrolled = $state(false);
	let query = $state('');

	const onImage = $derived(shell.overImage && !scrolled);

	function onScroll() {
		scrolled = window.scrollY > 12;
	}

	function submit(e: SubmitEvent) {
		e.preventDefault();
		const q = query.trim();
		goto(q ? `/recherche?q=${encodeURIComponent(q)}` : '/recherche');
	}

	$effect(onScroll);
</script>

<svelte:window onscroll={onScroll} />

<header class="header" class:scrolled class:on-image={onImage}>
	<a class="brand" href="/" aria-label="Aether — accueil"><span class="mark">A</span>ether</a>

	<form class="search" role="search" onsubmit={submit}>
		<SearchField bind:value={query} shortcut />
	</form>

	<div class="end">
		<a class="icon-link search-link" href="/recherche" aria-label="Rechercher"><Icon name="search" /></a>
		<ThemeSwitcher />
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
		margin-bottom: calc(-1 * var(--header-h));
		padding: 0 var(--gutter);
		background: transparent;
		transition:
			background-color var(--dur-base) var(--ease),
			box-shadow var(--dur-base) var(--ease);
	}

	.header.scrolled {
		background: var(--surface);
		box-shadow: 0 1px 0 var(--line), var(--shadow-1);
	}

	.brand {
		display: none;
		font-family: var(--font-display);
		font-size: 1.625rem;
		font-weight: 500;
		letter-spacing: 0.01em;
		color: var(--text);
		border-radius: var(--r-sm);
	}

	.mark {
		color: var(--accent);
		font-weight: 600;
	}

	.search {
		width: min(380px, 100%);
	}

	.end {
		display: flex;
		align-items: center;
		gap: var(--sp-1);
		margin-left: auto;
	}

	.icon-link {
		display: none;
		place-items: center;
		width: 40px;
		height: 40px;
		border-radius: var(--r-full);
		color: var(--text-muted);
	}

	/* Over a full-bleed image: constant light ink, matte dark field. */
	.on-image .brand {
		color: var(--on-image);
	}

	.on-image :global(.switcher .trigger),
	.on-image .icon-link {
		color: var(--on-image-muted);
	}

	.on-image :global(.switcher .trigger:hover) {
		color: var(--on-image);
		background: rgba(239, 232, 220, 0.1);
	}

	.on-image .search :global(.control) {
		background: rgba(12, 11, 10, 0.42);
		color: var(--on-image-muted);
		box-shadow: inset 0 0 0 1px rgba(239, 232, 220, 0.1);
	}

	.on-image .search :global(input) {
		color: var(--on-image);
	}

	.on-image .search :global(input::placeholder) {
		color: rgba(239, 232, 220, 0.6);
	}

	.on-image .search :global(kbd) {
		color: var(--on-image-muted);
		box-shadow: inset 0 0 0 1px rgba(239, 232, 220, 0.18);
	}

	@media (max-width: 767px) {
		.brand {
			display: block;
		}

		.search {
			display: none;
		}

		.icon-link {
			display: grid;
		}
	}
</style>
