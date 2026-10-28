<script lang="ts">
	import { page } from '$app/state';
	import { api, errorMessage, isAbort } from '$lib/api/client';
	import { libraries } from '$lib/api/stores.svelte';
	import type { Library } from '$lib/api/types';
	import ItemBrowser from '$lib/components/browse/ItemBrowser.svelte';
	import Button from '$lib/components/Button.svelte';
	import { session } from '$lib/session.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import { libraryKindInfo } from '$lib/media';

	const id = $derived(Number(page.params.id));
	let fetched = $state<Library | null>(null);
	let error = $state<unknown>(null);

	// The shared list usually has it; otherwise ask the server directly.
	const library = $derived(libraries.get(id) ?? (fetched?.id === id ? fetched : undefined));

	$effect(() => {
		const want = id;
		if (!libraries.loaded || libraries.get(want)) return;
		error = null;
		const ctrl = new AbortController();
		api
			.library(want, ctrl.signal)
			.then((l) => (fetched = l))
			.catch((err) => !isAbort(err) && (error = err));
		return () => ctrl.abort();
	});
</script>

<svelte:head>
	<title>{library?.name ?? 'Bibliothèque'} — Aether</title>
</svelte:head>

{#if library}
	{#key library.id}
		<ItemBrowser title={library.name} eyebrow={libraryKindInfo(library.kind).label === library.name ? "Bibliothèque" : libraryKindInfo(library.kind).label} {library} />
	{/key}
{:else if error}
	<div class="page">
		<EmptyState icon="alert" title="Bibliothèque introuvable" role="alert">
			<p>{errorMessage(error)}</p>
			{#snippet actions()}{#if session.isAdmin}<Button href="/reglages/bibliotheques">Gérer les bibliothèques</Button>{:else}<Button href="/" icon="home">Accueil</Button>{/if}{/snippet}
		</EmptyState>
	</div>
{/if}
