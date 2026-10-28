<script lang="ts">
	// One collection: its titles; manual ones can be renamed, shared (admins), emptied item by item, deleted.
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, errorMessage, isAbort } from '$lib/api/client';
	import type { Collection, Item } from '$lib/api/types';
	import Button from '$lib/components/Button.svelte';
	import CollectionDialog from '$lib/components/collections/CollectionDialog.svelte';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import ItemCard from '$lib/components/ItemCard.svelte';
	import Menu, { type MenuItem } from '$lib/components/Menu.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import Tag from '$lib/components/Tag.svelte';
	import { plural } from '$lib/format';
	import { session } from '$lib/session.svelte';
	import { toasts } from '$lib/toast.svelte';

	const id = $derived(page.params.id ?? '');

	let collection = $state<Collection | null>(null);
	let items = $state<Item[]>([]);
	let error = $state<unknown>(null);
	let editOpen = $state(false);
	let deleteOpen = $state(false);
	let deleting = $state(false);

	const canEdit = $derived(
		!!collection && collection.kind === 'manual' && (session.isAdmin || collection.ownerId === undefined || collection.ownerId === session.user?.id)
	);

	$effect(() => {
		const want = id;
		collection = null;
		items = [];
		error = null;
		const ctrl = new AbortController();
		api
			.collectionItems(want, ctrl.signal)
			.then((r) => {
				collection = r.collection;
				items = r.items;
			})
			.catch((err) => !isAbort(err) && (error = err));
		return () => ctrl.abort();
	});

	async function remove(it: Item) {
		if (!collection) return;
		const c = collection;
		const before = items;
		items = items.filter((x) => x.id !== it.id);
		c.count = Math.max(0, c.count - 1);
		try {
			await api.removeFromCollection(c.id, it.id);
			toasts.show(`« ${it.title} » retiré de la collection`, {
				action: {
					label: 'Annuler',
					run: async () => {
						try {
							await api.addToCollection(c.id, it.id);
							items = before;
							c.count = before.length;
						} catch (err) {
							toasts.show('Impossible de rétablir', { detail: errorMessage(err) });
						}
					}
				}
			});
		} catch (err) {
			items = before;
			c.count = before.length;
			toasts.show('Retrait impossible', { detail: errorMessage(err) });
		}
	}

	async function toggleShared() {
		if (!collection) return;
		const c = collection;
		const want = !c.shared;
		try {
			await api.patchCollection(c.id, { shared: want });
			c.shared = want;
			toasts.show(want ? 'Collection partagée avec tous les utilisateurs' : 'Collection rendue privée');
		} catch (err) {
			toasts.show('Modification impossible', { detail: errorMessage(err) });
		}
	}

	async function confirmDelete() {
		if (!collection) return;
		deleting = true;
		try {
			await api.deleteCollection(collection.id);
			deleteOpen = false;
			toasts.show(`Collection « ${collection.name} » supprimée`, { detail: 'Les titres eux-mêmes n’ont pas été touchés.' });
			goto('/collections', { replaceState: true });
		} catch (err) {
			toasts.show('Suppression impossible', { detail: errorMessage(err) });
		} finally {
			deleting = false;
		}
	}

	const menu = $derived<MenuItem[]>(
		canEdit && collection
			? [
					{ label: 'Renommer…', icon: 'edit', onselect: () => (editOpen = true) },
					...(session.isAdmin
						? [{ label: collection.shared ? 'Ne plus partager' : 'Partager avec tous', icon: 'users' as const, onselect: toggleShared }]
						: []),
					{ label: 'Supprimer la collection…', icon: 'trash', onselect: () => (deleteOpen = true), separated: true }
				]
			: []
	);
</script>

<svelte:head>
	<title>{collection?.name ?? 'Collection'} — Aether</title>
</svelte:head>

<div class="page">
	{#if error}
		<EmptyState icon="alert" title={(error as { status?: number }).status === 404 ? 'Collection introuvable' : 'Impossible d’afficher la collection'} role="alert">
			<p>{errorMessage(error)}</p>
			{#snippet actions()}<Button href="/collections" icon="chevron-left">Toutes les collections</Button>{/snippet}
		</EmptyState>
	{:else if !collection}
		<div aria-busy="true">
			<Skeleton width="280px" height="40px" radius="sm" />
			<ul class="grid sk-grid" role="list">
				{#each Array.from({ length: 6 }, (_, i) => i) as i (i)}<li><Skeleton ratio="2 / 3" radius="lg" /></li>{/each}
			</ul>
		</div>
	{:else}
		<header class="head">
			<div class="titles">
				<a class="back micro" href="/collections"><Icon name="chevron-left" size={14} />Collections</a>
				<h1>{collection.name}</h1>
				<p class="meta">
					<span class="tabular">{plural(collection.count, 'titre', 'titres')}</span>
					{#if collection.kind === 'tmdb'}<Tag variant="accent">Automatique · TMDB</Tag>{/if}
					{#if collection.kind === 'manual' && collection.shared}<Tag icon="users">Partagée</Tag>{/if}
				</p>
			</div>
			{#if menu.length}<Menu label="Actions de la collection" items={menu} text="Gérer" icon="settings" />{/if}
		</header>

		{#if !items.length}
			<EmptyState icon="collection" title="Cette collection est vide" role="status">
				<p>Depuis la fiche d’un film ou d’une série, ouvrez « Plus » puis « Ajouter à une collection… ».</p>
			</EmptyState>
		{:else}
			<ul class="grid" role="list">
				{#each items as it (it.id)}
					<li class="cell">
						<ItemCard item={it} />
						{#if canEdit}
							<button type="button" class="remove" aria-label="Retirer « {it.title} » de la collection" title="Retirer de la collection" onclick={() => remove(it)}>
								<Icon name="x" size={16} />
							</button>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	{/if}
</div>

{#if collection && canEdit}
	<CollectionDialog bind:open={editOpen} {collection} onsaved={(c) => collection && Object.assign(collection, { name: c.name, shared: c.shared })} />
	<ConfirmDialog bind:open={deleteOpen} title="Supprimer la collection ?" confirmLabel="Supprimer" danger busy={deleting} onconfirm={confirmDelete}>
		<p>La collection <strong>{collection.name}</strong> sera supprimée. Les titres qu’elle contient restent dans vos bibliothèques.</p>
	</ConfirmDialog>
{/if}

<style>
	.page {
		display: grid;
		gap: var(--sp-6);
		padding: var(--sp-2) var(--gutter) var(--sp-9);
	}

	.head {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: var(--sp-4);
		flex-wrap: wrap;
	}

	.titles {
		display: grid;
		gap: var(--sp-2);
		min-width: 0;
	}

	h1 {
		overflow-wrap: anywhere;
	}

	.back {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		width: fit-content;
		color: var(--accent-ink);
		text-transform: uppercase;
		letter-spacing: 0.1em;
		border-radius: var(--r-xs);
	}

	.meta {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: var(--sp-2) var(--sp-3);
		color: var(--text-muted);
		font-size: var(--fs-small);
	}

	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
		gap: var(--sp-5) var(--gap);
	}

	.sk-grid {
		margin-top: var(--sp-6);
	}

	.cell {
		position: relative;
	}

	.remove {
		position: absolute;
		top: 8px;
		left: 8px;
		z-index: 2;
		display: grid;
		place-items: center;
		width: 30px;
		height: 30px;
		border-radius: var(--r-sm);
		background: rgba(4, 14, 22, 0.72);
		color: var(--on-image);
		box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.14);
		opacity: 0;
		transition:
			opacity var(--dur-fast) var(--ease),
			background-color var(--dur-fast) var(--ease);
	}

	.cell:hover .remove,
	.cell:focus-within .remove,
	.remove:focus-visible {
		opacity: 1;
	}

	.remove:hover {
		background: rgba(4, 14, 22, 0.92);
	}

	/* No hover on touch screens: keep the control visible. */
	@media (hover: none) {
		.remove {
			opacity: 1;
		}
	}

	@media (max-width: 479px) {
		.grid {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}
	}
</style>
