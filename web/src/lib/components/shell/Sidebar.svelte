<script lang="ts">
	import { page } from '$app/state';
	import { discover, isActive, library, settings, type NavItem } from '$lib/nav';
	import Logo from '../brand/Logo.svelte';
	import Icon from '../Icon.svelte';

	interface Props {
		collapsed: boolean;
		ontoggle: () => void;
		version?: string;
	}

	let { collapsed, ontoggle, version }: Props = $props();
</script>

{#snippet link(item: NavItem)}
	{@const active = isActive(page.url.pathname, item.href)}
	<li>
		<a
			class="link"
			class:active
			href={item.href}
			aria-current={active ? 'page' : undefined}
			title={collapsed ? item.label : undefined}
		>
			<Icon name={item.icon} size={18} filled={active && item.icon === 'home'} />
			<span class="label">{item.label}</span>
		</a>
	</li>
{/snippet}

<aside class="sidebar" class:collapsed aria-label="Navigation principale">
	<a class="brand" href="/" aria-label="Aether — accueil">
		<Logo size={30} wordmark={!collapsed} />
	</a>

	<nav aria-label="Rubriques">
		<ul role="list">
			{#each discover as item (item.href)}{@render link(item)}{/each}
		</ul>

		<ul role="list" class="group" aria-label="Bibliothèque">
			{#each library as item (item.href)}{@render link(item)}{/each}
		</ul>
	</nav>

	<div class="bottom">
		<ul role="list">
			{@render link(settings)}
		</ul>

		<div class="foot">
			<p class="meta">
				<a href="/design">Système de design</a>
				{#if version}<span class="tabular">Aether v{version}</span>{/if}
			</p>
			<button
				class="collapse"
				type="button"
				aria-expanded={!collapsed}
				aria-label={collapsed ? 'Déplier la barre latérale' : 'Replier la barre latérale'}
				title={collapsed ? 'Déplier' : 'Replier'}
				onclick={ontoggle}
			>
				<Icon name="panel" size={18} />
			</button>
		</div>
	</div>
</aside>

<style>
	.sidebar {
		position: sticky;
		top: 0;
		display: flex;
		flex-direction: column;
		height: 100vh;
		height: 100dvh;
		padding: 0 var(--sp-4) var(--sp-4);
		overflow: hidden auto;
		scrollbar-width: none;
	}

	.brand {
		display: flex;
		align-items: center;
		height: var(--header-h);
		flex: none;
		padding: 0 6px;
		margin-bottom: var(--sp-1);
		border-radius: var(--r-sm);
	}

	ul {
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		gap: var(--sp-2);
	}

	.group {
		margin-top: var(--sp-6);
	}

	.link {
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		width: 100%;
		height: 40px;
		padding: 0 var(--sp-3);
		border-radius: var(--r-sm);
		background: color-mix(in srgb, var(--surface) 72%, transparent);
		color: color-mix(in srgb, var(--text) 82%, var(--bg));
		font-size: var(--fs-body);
		font-weight: var(--fw-medium);
		white-space: nowrap;
		transition:
			color var(--dur-fast) var(--ease),
			background-color var(--dur-fast) var(--ease);
	}

	.link :global(svg) {
		color: var(--text-muted);
		transition: color var(--dur-fast) var(--ease);
	}

	.link:hover {
		color: var(--text);
		background: var(--surface-raised);
	}

	.link:hover :global(svg) {
		color: var(--text);
	}

	.link.active {
		color: var(--text);
		background: var(--surface-active);
		font-weight: var(--fw-bold);
	}

	.link.active :global(svg) {
		color: var(--accent);
	}

	.label {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		transition: opacity var(--dur-base) var(--ease);
	}

	.bottom {
		margin-top: auto;
		padding-top: var(--sp-6);
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		gap: var(--sp-3);
	}

	.foot {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--sp-2);
		padding: var(--sp-2) 0 0 var(--sp-1);
	}

	.meta {
		display: grid;
		min-width: 0;
		font-size: var(--fs-micro);
		line-height: var(--lh-micro);
		color: var(--text-muted);
		white-space: nowrap;
		transition: opacity var(--dur-base) var(--ease);
	}

	.meta a {
		overflow: hidden;
		text-overflow: ellipsis;
		border-radius: var(--r-xs);
	}

	.meta a:hover {
		color: var(--text);
	}

	.collapse {
		display: grid;
		place-items: center;
		flex: none;
		width: 36px;
		height: 36px;
		border-radius: var(--r-sm);
		color: var(--text-muted);
		transition:
			color var(--dur-fast) var(--ease),
			background-color var(--dur-fast) var(--ease);
	}

	.collapse:hover {
		color: var(--text);
		background: var(--surface-raised);
	}

	/* Collapsed: icon tiles only. */
	.collapsed {
		padding-inline: 14px;
	}

	.collapsed .brand {
		padding: 0 5px;
	}

	.collapsed .link {
		padding: 0 14px;
	}

	.collapsed .label,
	.collapsed .meta {
		opacity: 0;
		pointer-events: none;
	}

	.collapsed .meta {
		display: none;
	}

	.collapsed .foot {
		justify-content: center;
		padding-left: 0;
	}
</style>
