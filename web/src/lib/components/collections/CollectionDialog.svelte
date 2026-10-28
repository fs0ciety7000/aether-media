<script lang="ts">
	// Create or rename a manual collection; admins can share it with every user.
	import { api, errorMessage } from '$lib/api/client';
	import type { Collection } from '$lib/api/types';
	import { session } from '$lib/session.svelte';
	import Button from '../Button.svelte';
	import Dialog from '../Dialog.svelte';
	import Input from '../Input.svelte';
	import Switch from '../Switch.svelte';

	interface Props {
		open: boolean;
		/** Edit this collection; create one otherwise. */
		collection?: Collection | null;
		onsaved?: (c: Collection, created: boolean) => void;
	}

	let { open = $bindable(false), collection = null, onsaved }: Props = $props();

	let name = $state('');
	let shared = $state(false);
	let busy = $state(false);
	let error = $state('');

	$effect(() => {
		if (!open) return;
		name = collection?.name ?? '';
		shared = collection?.shared ?? false;
		error = '';
	});

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		const n = name.trim();
		if (!n) {
			error = 'Donnez un nom à la collection.';
			return;
		}
		busy = true;
		error = '';
		try {
			const body = { name: n, ...(session.isAdmin ? { shared } : {}) };
			if (collection) {
				const c = (await api.patchCollection(collection.id, body)) ?? { ...collection, ...body };
				onsaved?.(c, false);
			} else {
				onsaved?.(await api.createCollection(body), true);
			}
			open = false;
		} catch (err) {
			error = errorMessage(err);
		} finally {
			busy = false;
		}
	}
</script>

<Dialog bind:open title={collection ? 'Modifier la collection' : 'Nouvelle collection'} size="sm" initialFocus="#col-name">
	<form id="col-form" class="form" onsubmit={submit} novalidate>
		<Input id="col-name" label="Nom" bind:value={name} maxlength={80} autocomplete="off" placeholder="Soirées d’hiver, Intégrale Miyazaki…" aria-invalid={!!error} />
		{#if session.isAdmin}
			<Switch label="Partagée" hint="Visible par tous les utilisateurs du serveur." bind:checked={shared} />
		{/if}
		{#if error}<p class="error" role="alert">{error}</p>{/if}
	</form>
	{#snippet footer()}
		<Button variant="tertiary" onclick={() => (open = false)}>Annuler</Button>
		<Button variant="primary" type="submit" form="col-form" disabled={busy}>{collection ? 'Enregistrer' : 'Créer'}</Button>
	{/snippet}
</Dialog>

<style>
	.form {
		display: grid;
		gap: var(--sp-5);
	}

	.error {
		color: var(--danger);
		font-size: var(--fs-small);
	}
</style>
