<script lang="ts">
	import { goto } from '$app/navigation';
	import { currentUser } from '$lib/demo/data';
	import { toasts } from '$lib/toast.svelte';
	import Logo from '../brand/Logo.svelte';
	import IconButton from '../IconButton.svelte';
	import SearchField from '../SearchField.svelte';
	import ThemeSwitcher from '../ThemeSwitcher.svelte';
	import UserChip from '../UserChip.svelte';

	let scrolled = $state(false);
	let query = $state('');

	function onScroll() {
		scrolled = window.scrollY > 8;
	}

	function submit(e: SubmitEvent) {
		e.preventDefault();
		const q = query.trim();
		goto(q ? `/recherche?q=${encodeURIComponent(q)}` : '/recherche');
	}

	$effect(onScroll);
</script>

<svelte:window onscroll={onScroll} />

<header class="header" class:scrolled>
	<a class="brand" href="/" aria-label="Aether — accueil"><Logo size={26} /></a>

	<form class="search" role="search" onsubmit={submit}>
		<SearchField bind:value={query} size="lg" shortcut placeholder="Rechercher un film, un réalisateur, un acteur…" />
	</form>

	<div class="end">
		<ThemeSwitcher />
		<IconButton
			icon="bell"
			badge
			aria-label="Notifications (2 nouvelles)"
			onclick={() => toasts.show('2 nouveaux titres', { detail: 'La bibliothèque « Films » a été analysée.' })}
		/>
		<span class="user-full"><UserChip name={currentUser.name} detail={currentUser.email} hue={currentUser.hue} /></span>
		<span class="user-compact"><UserChip name={currentUser.name} hue={currentUser.hue} compact /></span>
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
