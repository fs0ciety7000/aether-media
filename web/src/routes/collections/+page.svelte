<script lang="ts">
	// Collections: manual ones (yours and shared) and automatic ones from TMDB (Item.collection).
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api, errorMessage, isAbort } from '$lib/api/client';
	import type { Collection } from '$lib/api/types';
	import Button from '$lib/components/Button.svelte';
	import CollectionCard from '$lib/components/collections/CollectionCard.svelte';
	import CollectionDialog from '$lib/components/collections/CollectionDialog.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import SectionHeader from '$lib/components/SectionHeader.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import { plural } from '$lib/format';
	import { toasts } from '$lib/toast.svelte';

	let list = $state<Collection[] | null>(null);
	let error = $state<unknown>(null);
	let createOpen = $state(false);

	const manual = $derived((list ?? []).filter((c) => c.kind === 'manual'));
	const auto = $derived((list ?? []).filter((c) => c.kind === 'tmdb'));

	async function load() {
		error = null;
		try {
			list = await api.collections();
		} catch (err) {
			if (!isAbort(err)) error = err;
		}
	}

	onMount(load);

	function created(c: Collection) {
		toasts.show(`Collection « ${c.name} » créée`, { detail: 'Ajoutez-y des titres depuis leur fiche (menu « Plus »).' });
		goto(`/collections/${encodeURIComponent(c.id)}`);
	}
</script>

<svelte:head>
	<title>Collections — Aether</title>
</svelte:head>

<div class="page">
	<header class="head">
		<div>
			<h1>Collections</h1>
			{#if list}<p class="count tabular">{plural(list.length, 'collection', 'collections')}</p>{/if}
		</div>
		<Button variant="primary" icon="plus" onclick={() => (createOpen = true)}>Nouvelle collection</Button>
	</header>

	{#if error && !list}
		<EmptyState icon="alert" title="Impossible de charger les collections" role="alert">
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
		<EmptyState icon="collection" title="Aucune collection pour l’instant" role="status">
			<p>Regroupez vos titres comme vous l’entendez : une saga, un cycle, une soirée. Les sagas connues de TMDB apparaissent ici automatiquement.</p>
			{#snippet actions()}<Button variant="primary" icon="plus" onclick={() => (createOpen = true)}>Créer une collection</Button>{/snippet}
		</EmptyState>
	{:else}
		{#if manual.length}
			<section aria-labelledby="col-manual">
				<SectionHeader title="Mes collections" id="col-manual" />
				<ul class="grid" role="list">
					{#each manual as c (c.id)}<li><CollectionCard collection={c} /></li>{/each}
				</ul>
			</section>
		{/if}
		{#if auto.length}
			<section aria-labelledby="col-auto">
				<SectionHeader title="Sagas et collections automatiques" id="col-auto" />
				<ul class="grid" role="list">
					{#each auto as c (c.id)}<li><CollectionCard collection={c} /></li>{/each}
				</ul>
			</section>
		{/if}
	{/if}
</div>

<CollectionDialog bind:open={createOpen} onsaved={created} />

<style>
	.page {
		display: grid;
		gap: var(--sp-7);
		padding: var(--sp-2) var(--gutter) var(--sp-9);
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

	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
		gap: var(--sp-5) var(--gap);
	}

	.sk {
		display: grid;
		gap: 10px;
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

		.head :global(.btn) {
			width: 100%;
		}
	}
</style>
