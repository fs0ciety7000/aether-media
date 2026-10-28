<script lang="ts">
	// Ma liste: the user's watchlist (films and series), most recent additions first.
	// Synchronised with the Trakt watchlist / Simkl "Plan to Watch" when the user enabled it.
	import { onMount } from 'svelte';
	import { api, errorMessage, isAbort } from '$lib/api/client';
	import { events } from '$lib/api/events.svelte';
	import type { Item, UserDataChangedEvent } from '$lib/api/types';
	import Button from '$lib/components/Button.svelte';
	import Chip from '$lib/components/Chip.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import ItemCard from '$lib/components/ItemCard.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import { plural } from '$lib/format';
	import { session } from '$lib/session.svelte';
	import { toasts } from '$lib/toast.svelte';

	let list = $state<Item[] | null>(null);
	let error = $state<unknown>(null);
	let kind = $state<'all' | 'movie' | 'series'>('all');

	const movies = $derived((list ?? []).filter((i) => i.kind === 'movie').length);
	const series = $derived((list ?? []).filter((i) => i.kind === 'series').length);
	const shown = $derived((list ?? []).filter((i) => kind === 'all' || i.kind === kind));

	async function load() {
		try {
			list = await api.watchlist();
			error = null;
		} catch (err) {
			if (!isAbort(err)) error = err;
		}
	}

	/** Our own removals also come back as userdata.changed. */
	let lastOwnChange = 0;

	onMount(() => {
		load();
		const off = events.debounced(
			'userdata.changed',
			() => load(),
			1200,
			(ev) => {
				const d = ev.data as UserDataChangedEvent | undefined;
				return !!d && (!session.user || d.userId === session.user.id) && Date.now() - lastOwnChange > 1500;
			}
		);
		const onVisible = () => document.visibilityState === 'visible' && load();
		document.addEventListener('visibilitychange', onVisible);
		return () => {
			off();
			document.removeEventListener('visibilitychange', onVisible);
		};
	});

	async function remove(it: Item) {
		if (!list) return;
		const index = list.findIndex((x) => x.id === it.id);
		list = list.filter((x) => x.id !== it.id);
		lastOwnChange = Date.now();
		try {
			await api.removeFromWatchlist(it.id);
			lastOwnChange = Date.now();
			toasts.show('Retiré de « Ma liste »', { detail: it.title, duration: 5000, action: { label: 'Annuler', run: () => restore(it, index) } });
		} catch (err) {
			list = [...(list ?? []).slice(0, index), it, ...(list ?? []).slice(index)];
			toasts.show('Modification impossible', { detail: errorMessage(err) });
		}
	}

	async function restore(it: Item, index: number) {
		lastOwnChange = Date.now();
		try {
			await api.addToWatchlist(it.id);
			// Back where it was on screen (the server now dates it as the newest addition).
			const cur = (list ?? []).filter((x) => x.id !== it.id);
			list = [...cur.slice(0, index), it, ...cur.slice(index)];
		} catch (err) {
			toasts.show('Modification impossible', { detail: errorMessage(err) });
		}
	}
</script>

<svelte:head>
	<title>Ma liste — Aether</title>
</svelte:head>

<div class="page">
	<header class="head">
		<div>
			<h1>Ma liste</h1>
			{#if list}<p class="count tabular" aria-live="polite">{plural(list.length, 'titre', 'titres')} à voir</p>{/if}
		</div>
		{#if list?.length && movies && series}
			<div class="filters" role="group" aria-label="Type">
				<Chip toggle={false} selected={kind === 'all'} onclick={() => (kind = 'all')}>Tout</Chip>
				<Chip toggle={false} selected={kind === 'movie'} count={movies} onclick={() => (kind = 'movie')}>Films</Chip>
				<Chip toggle={false} selected={kind === 'series'} count={series} onclick={() => (kind = 'series')}>Séries</Chip>
			</div>
		{/if}
	</header>

	{#if error && !list}
		<EmptyState icon="alert" title="Impossible de charger votre liste" role="alert">
			<p>{errorMessage(error)}</p>
			{#snippet actions()}<Button icon="refresh" onclick={load}>Réessayer</Button>{/snippet}
		</EmptyState>
	{:else if !list}
		<ul class="grid" role="list" aria-busy="true">
			{#each Array.from({ length: 8 }, (_, i) => i) as i (i)}
				<li class="sk"><Skeleton ratio="2 / 3" radius="lg" /><Skeleton width="60%" height="12px" radius="sm" /></li>
			{/each}
		</ul>
	{:else if !list.length}
		<EmptyState icon="bookmark" title="Votre liste est vide" role="status">
			<p>Gardez ici les films et les séries à voir plus tard : bouton « Ma liste » sur leur fiche. Si vous avez relié Trakt ou Simkl, leur watchlist arrive ici aussi.</p>
			{#snippet actions()}
				<Button href="/parcourir" icon="compass">Parcourir</Button>
				<Button href="/profil/synchronisation" variant="tertiary" icon="refresh">Synchronisation</Button>
			{/snippet}
		</EmptyState>
	{:else}
		<ul class="grid" role="list">
			{#each shown as it (it.id)}
				<li class="cell">
					<ItemCard item={it} sizes="(max-width: 479px) 45vw, (max-width: 1023px) 30vw, 180px" />
					<button type="button" class="remove" onclick={() => remove(it)} aria-label="Retirer « {it.title} » de Ma liste" title="Retirer de Ma liste">
						<Icon name="x" size={16} />
					</button>
				</li>
			{/each}
		</ul>
	{/if}
</div>

<style>
	.page {
		display: grid;
		gap: var(--sp-6);
		align-content: start;
		padding-top: var(--sp-2);
	}

	.head {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: var(--sp-4);
		flex-wrap: wrap;
	}

	.count {
		margin-top: 2px;
		color: var(--text-muted);
		font-size: var(--fs-small);
	}

	.filters {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
	}

	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
		gap: var(--sp-5) var(--gap);
	}

	.sk {
		display: grid;
		gap: 10px;
	}

	.cell {
		position: relative;
		min-width: 0;
	}

	.remove {
		position: absolute;
		/* Top left: the watch badge sits top right. */
		top: 8px;
		left: 8px;
		z-index: 2;
		display: grid;
		place-items: center;
		width: 32px;
		height: 32px;
		border-radius: 50%;
		background: rgba(4, 14, 22, 0.72);
		color: var(--on-image);
		box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.14);
		opacity: 0;
		transition:
			opacity var(--dur-fast) var(--ease),
			background-color var(--dur-fast) var(--ease);
	}

	.cell:hover .remove,
	.remove:focus-visible {
		opacity: 1;
	}

	.remove:hover {
		background: rgba(4, 14, 22, 0.9);
	}

	@media (hover: none) {
		.remove {
			opacity: 1;
		}
	}

	@media (min-width: 1600px) {
		.grid {
			grid-template-columns: repeat(auto-fill, minmax(168px, 1fr));
		}
	}

	@media (max-width: 479px) {
		.grid {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}
	}
</style>
