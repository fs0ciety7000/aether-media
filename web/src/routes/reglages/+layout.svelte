<script lang="ts">
	// Settings are for administrators: sub-navigation (Bibliothèques · Utilisateurs · Synchronisation · Serveur · Tâches · Journaux · Lectures) above each page.
	import Button from '$lib/components/Button.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import SubNav from '$lib/components/shell/SubNav.svelte';
	import { settingsNav } from '$lib/nav';
	import { session } from '$lib/session.svelte';

	let { children } = $props();
</script>

{#if session.isAdmin}
	<SubNav items={settingsNav} label="Réglages" />
	{@render children()}
{:else}
	<div class="page">
		<EmptyState icon="lock" title="Réservé aux administrateurs" role="status">
			<p>Les bibliothèques, les comptes et les services de synchronisation sont gérés par un administrateur de ce serveur.</p>
			{#snippet actions()}<Button href="/profil" icon="user">Mon profil</Button>{/snippet}
		</EmptyState>
	</div>
{/if}
