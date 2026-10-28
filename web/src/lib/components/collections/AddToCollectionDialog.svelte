<script lang="ts">
	// "Ajouter à une collection…": pick one of your manual collections, or create one on the spot.
	import { goto } from '$app/navigation';
	import { api, errorMessage } from '$lib/api/client';
	import type { Collection, Item } from '$lib/api/types';
	import { plural } from '$lib/format';
	import { session } from '$lib/session.svelte';
	import { toasts } from '$lib/toast.svelte';
	import Button from '../Button.svelte';
	import Dialog from '../Dialog.svelte';
	import Icon from '../Icon.svelte';
	import Input from '../Input.svelte';
	import Skeleton from '../Skeleton.svelte';

	interface Props {
		open: boolean;
		item: Pick<Item, 'id' | 'title'>;
	}

	let { open = $bindable(false), item }: Props = $props();

	let list = $state<Collection[] | null>(null);
	let loadError = $state('');
	let busy = $state<string | null>(null);
	let name = $state('');
	let error = $state('');

	/** Collections this user may fill: their own, or any manual one for admins. */
	const editable = $derived((list ?? []).filter((c) => c.kind === 'manual' && (session.isAdmin || c.ownerId === undefined || c.ownerId === session.user?.id)));

	$effect(() => {
		if (!open) return;
		list = null;
		loadError = '';
		name = '';
		error = '';
		const ctrl = new AbortController();
		api
			.collections(ctrl.signal)
			.then((c) => (list = c))
			.catch((err) => {
				list = [];
				loadError = errorMessage(err);
			});
		return () => ctrl.abort();
	});

	async function add(c: Collection) {
		busy = c.id;
		try {
			await api.addToCollection(c.id, item.id);
			open = false;
			toasts.show(`Ajouté à « ${c.name} »`, { detail: item.title, action: { label: 'Voir', run: () => goto(`/collections/${encodeURIComponent(c.id)}`) } });
		} catch (err) {
			error = errorMessage(err);
		} finally {
			busy = null;
		}
	}

	async function createAndAdd(e: SubmitEvent) {
		e.preventDefault();
		const n = name.trim();
		if (!n) {
			error = 'Donnez un nom à la nouvelle collection.';
			return;
		}
		busy = 'new';
		error = '';
		try {
			const c = await api.createCollection({ name: n });
			await add(c);
		} catch (err) {
			error = errorMessage(err);
			busy = null;
		}
	}
</script>

<Dialog bind:open title="Ajouter à une collection" description="« {item.title} »" size="sm">
	{#if list === null}
		<div class="sk" aria-busy="true">
			{#each [0, 1, 2] as i (i)}<Skeleton height="48px" radius="md" />{/each}
		</div>
	{:else}
		{#if loadError}<p class="error" role="alert">{loadError}</p>{/if}
		{#if editable.length}
			<ul class="list" role="list" aria-label="Vos collections">
				{#each editable as c (c.id)}
					<li>
						<button type="button" class="row" disabled={!!busy} onclick={() => add(c)}>
							<span class="ico" aria-hidden="true"><Icon name="collection" size={18} /></span>
							<span class="text">
								<span class="name">{c.name}</span>
								<span class="meta">{plural(c.count, 'titre', 'titres')}{c.shared ? ' · partagée' : ''}</span>
							</span>
							<Icon name={busy === c.id ? 'refresh' : 'plus'} size={18} />
						</button>
					</li>
				{/each}
			</ul>
		{:else if !loadError}
			<p class="muted">Vous n’avez pas encore de collection. Créez-en une :</p>
		{/if}

		<form class="new" onsubmit={createAndAdd} novalidate>
			<Input label="Nouvelle collection" bind:value={name} maxlength={80} autocomplete="off" placeholder="Nom de la collection" />
			<Button type="submit" icon="plus" disabled={!!busy}>Créer et ajouter</Button>
		</form>
		{#if error}<p class="error" role="alert">{error}</p>{/if}
	{/if}
</Dialog>

<style>
	.sk,
	.list {
		display: grid;
		gap: var(--sp-2);
	}

	.row {
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		width: 100%;
		min-height: 52px;
		padding: var(--sp-2) var(--sp-3);
		border-radius: var(--r-md);
		background: var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
		color: var(--text-muted);
		text-align: left;
		transition: background-color var(--dur-fast) var(--ease);
	}

	.row:hover:not(:disabled) {
		background: var(--surface-active);
		color: var(--text);
	}

	.ico {
		display: grid;
		place-items: center;
		flex: none;
		width: 34px;
		height: 34px;
		border-radius: var(--r-sm);
		background: var(--accent-soft);
		color: var(--accent-ink);
	}

	.text {
		display: grid;
		flex: 1;
		min-width: 0;
	}

	.name {
		font-weight: var(--fw-bold);
		color: var(--text);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.meta {
		font-size: var(--fs-micro);
	}

	.new {
		display: grid;
		grid-template-columns: minmax(0, 1fr) auto;
		align-items: end;
		gap: var(--sp-3);
		margin-top: var(--sp-5);
		padding-top: var(--sp-4);
		box-shadow: 0 -1px 0 var(--line);
	}

	.muted {
		color: var(--text-muted);
		font-size: var(--fs-small);
	}

	.error {
		margin-top: var(--sp-3);
		color: var(--danger);
		font-size: var(--fs-small);
	}

	@media (max-width: 479px) {
		.new {
			grid-template-columns: minmax(0, 1fr);
		}
	}
</style>
