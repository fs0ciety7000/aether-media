<script lang="ts">
	import { page } from '$app/state';
	import Button from '$lib/components/Button.svelte';

	const notFound = $derived(page.status === 404);
</script>

<svelte:head>
	<title>{notFound ? 'Bientôt' : 'Erreur'} — Aether</title>
</svelte:head>

<section class="page error">
	<p class="micro code tabular">{notFound ? 'Entracte' : `Erreur ${page.status}`}</p>
	<h1>{notFound ? 'Cette salle n’ouvre pas encore.' : 'La projection s’est interrompue.'}</h1>
	<p class="lead">
		{#if notFound}
			La page <span class="path">{page.url.pathname}</span> arrivera dans une prochaine version d’Aether.
		{:else}
			{page.error?.message ?? 'Une erreur inattendue est survenue.'}
		{/if}
	</p>
	<Button variant="secondary" href="/" icon="chevron-left">Retour à l’accueil</Button>
</section>

<style>
	.error {
		display: grid;
		justify-items: start;
		align-content: center;
		gap: var(--sp-4);
		min-height: 70vh;
		max-width: calc(640px + 2 * var(--gutter));
	}

	.code {
		color: var(--accent-ink);
	}

	h1 {
		font-size: clamp(2.25rem, 4vw, 3rem);
		line-height: 1.1;
	}

	.lead {
		color: var(--text-muted);
		margin-bottom: var(--sp-4);
		max-width: 52ch;
	}

	.path {
		color: var(--text);
		font-variant-numeric: tabular-nums;
	}
</style>
