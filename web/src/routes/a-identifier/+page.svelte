<script lang="ts">
	// Items the automatic matching could not identify: fix them by hand.
	import { onMount } from 'svelte';
	import { api, errorMessage, isAbort } from '$lib/api/client';
	import { events } from '$lib/api/events.svelte';
	import { firstImage } from '$lib/api/images';
	import { IDENTIFIABLE_KINDS, libraries, system, unmatched } from '$lib/api/stores.svelte';
	import type { Item } from '$lib/api/types';
	import Artwork from '$lib/components/Artwork.svelte';
	import Button from '$lib/components/Button.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import IdentifyDialog from '$lib/components/items/IdentifyDialog.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import { plural } from '$lib/format';
	import { isSquare, KIND_ICON, KIND_LABEL } from '$lib/media';

	let items = $state<Item[]>([]);
	let total = $state<number | undefined>();
	let cursor = $state<string | undefined>();
	let loading = $state(true);
	let error = $state<unknown>(null);
	let target = $state<Item | null>(null);
	let dialogOpen = $state(false);

	const tmdbOff = $derived(system.info?.metadata?.tmdb === false);

	async function load(more = false) {
		if (!more) loading = true;
		try {
			const p = await api.items({ identify: 'unmatched', kind: IDENTIFIABLE_KINDS, sort: 'title', limit: 100, cursor: more ? cursor : undefined });
			items = more ? [...items, ...p.items] : p.items;
			total = p.total;
			cursor = p.nextCursor;
			error = null;
		} catch (err) {
			if (!isAbort(err)) error = err;
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		system.ensure();
		load();
		return events.debounced(['item.updated', 'library.scan.finished'], () => load(), 1500);
	});

	function open(it: Item) {
		target = it;
		dialogOpen = true;
	}

	function identified(it: Item) {
		items = items.filter((x) => x.id !== it.id);
		if (total) total--;
		unmatched.refresh();
	}

	const label = (it: Item) => KIND_LABEL[it.kind].one.replace(/^./, (c) => c.toUpperCase());
</script>

<svelte:head>
	<title>À identifier — Aether</title>
</svelte:head>

<section class="page identify">
	<header class="head">
		<div>
			<h1>À identifier</h1>
			<p class="lead">
				{#if total !== undefined && total > 0}
					{plural(total, 'titre n’a pas été reconnu', 'titres n’ont pas été reconnus')} automatiquement.
				{:else}
					Titres que la reconnaissance automatique n’a pas su associer à une fiche.
				{/if}
			</p>
		</div>
	</header>

	{#if tmdbOff}
		<aside class="banner" aria-labelledby="tmdb-title">
			<span class="b-icon" aria-hidden="true"><Icon name="key" size={20} /></span>
			<div class="b-text">
				<h2 id="tmdb-title">TMDB n’est pas configuré</h2>
				<p>
					Sans clé API, films et séries ne sont décrits que par les NFO et images locales, et la recherche de correspondances
					n’est pas disponible. Ajoutez une clé (v3) ou un jeton de lecture (v4) obtenu sur
					<a href="https://www.themoviedb.org/settings/api" target="_blank" rel="noreferrer">themoviedb.org</a>, puis redémarrez le serveur :
				</p>
				<ul role="list" class="b-options">
					<li><span>Dans <code>aether.toml</code></span><code class="block">[metadata]<br />tmdb_api_key = "…"</code></li>
					<li><span>Ou par variable d’environnement</span><code class="block">AETHER_TMDB_API_KEY=…</code></li>
				</ul>
			</div>
		</aside>
	{/if}

	{#if loading}
		<ul class="list" role="list" aria-busy="true">
			{#each [0, 1, 2] as i (i)}
				<li class="row sk"><Skeleton width="56px" ratio="2 / 3" radius="sm" /><Skeleton lines={2} width="60%" /></li>
			{/each}
		</ul>
	{:else if error}
		<EmptyState icon="alert" title="Liste indisponible" role="alert">
			<p>{errorMessage(error)}</p>
			{#snippet actions()}<Button icon="refresh" onclick={() => load()}>Réessayer</Button>{/snippet}
		</EmptyState>
	{:else if !items.length}
		<EmptyState icon="check" title="Tout est identifié" role="status">
			<p>Chaque film, série, artiste et album est associé à une fiche. Les nouveaux titres non reconnus apparaîtront ici.</p>
		</EmptyState>
	{:else}
		<ul class="list" role="list">
			{#each items as it (it.id)}
				<li class="row">
					<a class="thumb" class:square={isSquare(it.kind)} href="/item/{it.id}" tabindex="-1" aria-hidden="true">
						<Artwork image={firstImage(it, 'poster')} ratio={isSquare(it.kind) ? '1 / 1' : '2 / 3'} sizes="64px" placeholder={{ title: it.title, icon: KIND_ICON[it.kind] }} />
					</a>
					<div class="text">
						<a class="title" href="/item/{it.id}">{it.title}{#if it.year}<span class="year tabular"> ({it.year})</span>{/if}</a>
						<span class="meta">{label(it)}{#if libraries.get(it.libraryId)} · {libraries.get(it.libraryId)?.name}{/if}</span>
						{#if it.path}<span class="path" title={it.path}>{it.path}</span>{/if}
					</div>
					<Button variant="secondary" icon="wand" onclick={() => open(it)} aria-label="Identifier {it.title}">Identifier</Button>
				</li>
			{/each}
		</ul>
		{#if cursor}<div class="more"><Button onclick={() => load(true)}>Afficher plus</Button></div>{/if}
	{/if}
</section>

{#if target}
	<IdentifyDialog bind:open={dialogOpen} item={target} onidentified={identified} />
{/if}

<style>
	.identify {
		display: grid;
		gap: var(--sp-5);
		align-content: start;
		max-width: 1100px;
	}

	.lead {
		margin-top: var(--sp-1);
		color: var(--text-muted);
	}

	.banner {
		display: flex;
		gap: var(--sp-4);
		padding: var(--sp-5);
		border-radius: var(--r-xl);
		background: var(--tint), var(--surface);
		box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--accent) 26%, transparent);
	}

	.b-icon {
		display: grid;
		place-items: center;
		flex: none;
		width: 44px;
		height: 44px;
		border-radius: var(--r-md);
		background: var(--accent-soft);
		color: var(--accent-ink);
	}

	.b-text {
		display: grid;
		gap: var(--sp-2);
		min-width: 0;
		color: var(--text-muted);
	}

	.b-text h2 {
		font-size: var(--fs-h3);
		color: var(--text);
	}

	.b-text a {
		color: var(--accent-ink);
		text-decoration: underline;
		text-underline-offset: 3px;
	}

	.b-options {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(100%, 240px), 1fr));
		gap: var(--sp-3);
		margin-top: var(--sp-2);
	}

	.b-options li {
		display: grid;
		align-content: start;
		gap: var(--sp-1);
		font-size: var(--fs-small);
	}

	code {
		font-family: ui-monospace, 'SFMono-Regular', Menlo, monospace;
		font-size: 0.8125rem;
		color: var(--text);
	}

	code.block {
		display: block;
		padding: var(--sp-3);
		border-radius: var(--r-md);
		background: var(--bg-sunken);
		box-shadow: inset 0 0 0 1px var(--line);
		overflow-x: auto;
	}

	.list {
		display: grid;
		gap: var(--sp-2);
	}

	.row {
		display: flex;
		align-items: center;
		gap: var(--sp-4);
		padding: var(--sp-3);
		border-radius: var(--r-lg);
		background: var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
	}

	.thumb {
		flex: none;
		width: 56px;
		border-radius: var(--r-sm);
		overflow: hidden;
	}

	.thumb.square {
		width: 64px;
	}

	.text {
		display: grid;
		gap: 2px;
		flex: 1;
		min-width: 0;
	}

	.title {
		font-weight: var(--fw-bold);
		font-size: 0.9375rem;
	}

	.title:hover {
		text-decoration: underline;
		text-underline-offset: 3px;
	}

	.year {
		color: var(--text-muted);
		font-weight: var(--fw-medium);
	}

	.meta {
		font-size: var(--fs-small);
		color: var(--text-muted);
	}

	.path {
		font-size: var(--fs-micro);
		color: var(--text-faint);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.more {
		display: flex;
		justify-content: center;
	}

	@media (max-width: 767px) {
		.row {
			flex-wrap: wrap;
		}

		.row :global(.btn) {
			width: 100%;
		}

		.banner {
			flex-direction: column;
			padding: var(--sp-4);
		}
	}
</style>
