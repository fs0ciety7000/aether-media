<script lang="ts">
	// Renders its content for administrators; members get a calm explanation instead.
	import type { Snippet } from 'svelte';
	import { session } from '$lib/session.svelte';
	import Button from './Button.svelte';
	import EmptyState from './EmptyState.svelte';

	let { children }: { children: Snippet } = $props();
</script>

{#if session.isAdmin}
	{@render children()}
{:else}
	<div class="page">
		<EmptyState icon="lock" title="Réservé aux administrateurs" role="status">
			<p>Cette partie d’Aether est gérée par un administrateur de ce serveur.</p>
			{#snippet actions()}<Button href="/" icon="home">Retour à l’accueil</Button>{/snippet}
		</EmptyState>
	</div>
{/if}
