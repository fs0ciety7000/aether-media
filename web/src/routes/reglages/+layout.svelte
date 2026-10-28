<script lang="ts">
	// Settings are for administrators: sub-navigation (Bibliothèques · Utilisateurs) above each page.
	import { page } from '$app/state';
	import Button from '$lib/components/Button.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import { isActive, settingsNav } from '$lib/nav';
	import { session } from '$lib/session.svelte';

	let { children } = $props();
</script>

{#if session.isAdmin}
	<nav class="settings-nav" aria-label="Réglages">
		<ul role="list">
			{#each settingsNav as item (item.href)}
				{@const active = isActive(page.url.pathname, item.href)}
				<li>
					<a href={item.href} class:active aria-current={active ? 'page' : undefined}>
						<Icon name={item.icon} size={16} />{item.label}
					</a>
				</li>
			{/each}
		</ul>
	</nav>
	{@render children()}
{:else}
	<div class="page">
		<EmptyState icon="lock" title="Réservé aux administrateurs" role="status">
			<p>Les bibliothèques et les comptes sont gérés par un administrateur de ce serveur.</p>
			{#snippet actions()}<Button href="/profil" icon="user">Mon profil</Button>{/snippet}
		</EmptyState>
	</div>
{/if}

<style>
	.settings-nav {
		padding: var(--sp-2) var(--gutter) 0;
	}

	ul {
		display: inline-flex;
		gap: 4px;
		padding: 4px;
		border-radius: var(--r-full);
		background: var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
		max-width: 100%;
		overflow-x: auto;
		scrollbar-width: none;
	}

	a {
		display: inline-flex;
		align-items: center;
		gap: var(--sp-2);
		height: 34px;
		padding: 0 var(--sp-4);
		border-radius: var(--r-full);
		color: var(--text-muted);
		font-size: var(--fs-small);
		font-weight: var(--fw-bold);
		white-space: nowrap;
		transition:
			background-color var(--dur-fast) var(--ease),
			color var(--dur-fast) var(--ease);
	}

	a:hover {
		color: var(--text);
	}

	a.active {
		background: var(--surface-active);
		color: var(--text);
	}

	a.active :global(svg) {
		color: var(--accent);
	}
</style>
