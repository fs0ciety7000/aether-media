<script lang="ts">
	// Cross-library browsing: a genre (from the home categories) or a person (from search).
	import { page } from '$app/state';
	import ItemBrowser from '$lib/components/browse/ItemBrowser.svelte';

	const person = $derived(Number(page.url.searchParams.get('personne')) || undefined);
	const name = $derived(page.url.searchParams.get('nom') ?? undefined);
	const genre = $derived(page.url.searchParams.get('genre') ?? undefined);
	const title = $derived(person ? (name ?? 'Filmographie') : (genre ?? 'Tous les titres'));
	const eyebrow = $derived(person ? 'Avec' : genre ? 'Genre' : 'Parcourir');
</script>

<svelte:head>
	<title>{title} — Aether</title>
</svelte:head>

{#key person}
	<ItemBrowser {title} {eyebrow} base={{ person }} />
{/key}
