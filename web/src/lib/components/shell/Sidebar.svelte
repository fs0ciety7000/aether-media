<script lang="ts">
	import { page } from '$app/state';
	import { account, home, isActive, libraries, search, type NavItem } from '$lib/nav';
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
			<Icon name={item.icon} />
			<span class="label">{item.label}</span>
		</a>
	</li>
{/snippet}

<aside class="sidebar" class:collapsed aria-label="Navigation principale">
	<a class="brand" href="/" aria-label="Aether — accueil">
		<span class="mark" aria-hidden="true">A</span>
		<span class="word" aria-hidden="true">ether</span>
	</a>

	<nav>
		<ul role="list">
			{@render link(home)}
		</ul>

		<p class="section micro">Bibliothèques</p>
		<ul role="list">
			{#each libraries as item (item.href)}{@render link(item)}{/each}
		</ul>

		<hr />
		<ul role="list">
			{@render link(search)}
		</ul>
	</nav>

	<div class="bottom">
		<ul role="list">
			{#each account as item (item.href)}{@render link(item)}{/each}
		</ul>

		<button
			class="link collapse"
			type="button"
			aria-expanded={!collapsed}
			aria-label={collapsed ? 'Déplier la barre latérale' : 'Replier la barre latérale'}
			title={collapsed ? 'Déplier' : undefined}
			onclick={ontoggle}
		>
			<Icon name="panel" />
			<span class="label">Replier</span>
		</button>

		<p class="foot micro">
			<a href="/design">Système de design</a>
			{#if version}<span class="tabular">v{version}</span>{/if}
		</p>
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
		padding: 0 var(--sp-4) var(--sp-5);
		border-right: 1px solid var(--line);
		background: var(--bg);
		overflow: hidden auto;
		scrollbar-width: none;
	}

	.brand {
		display: flex;
		align-items: baseline;
		height: var(--header-h);
		flex: none;
		padding: 0 14px;
		margin-bottom: var(--sp-4);
		font-family: var(--font-display);
		font-size: 1.875rem;
		font-weight: 500;
		letter-spacing: 0.01em;
		line-height: var(--header-h);
		color: var(--text);
		border-radius: var(--r-md);
	}

	.mark {
		color: var(--accent);
		font-weight: 600;
	}

	.word,
	.label,
	.section,
	.foot {
		transition: opacity var(--dur-base) var(--ease);
		white-space: nowrap;
	}

	ul {
		display: grid;
		gap: 2px;
	}

	.section {
		padding: var(--sp-6) 14px var(--sp-3);
		color: var(--text-muted);
	}

	hr {
		border: 0;
		height: 1px;
		background: var(--line);
		margin: var(--sp-5) 14px;
	}

	.link {
		position: relative;
		display: flex;
		align-items: center;
		gap: 14px;
		width: 100%;
		height: 44px;
		padding: 0 14px;
		border-radius: var(--r-md);
		color: var(--text-muted);
		font-size: 0.9375rem;
		font-weight: 500;
		text-align: left;
		transition:
			color var(--dur-fast) var(--ease),
			background-color var(--dur-fast) var(--ease);
	}

	.link:hover {
		color: var(--text);
		background: color-mix(in srgb, var(--text) 5%, transparent);
	}

	.link.active {
		color: var(--text);
		background: color-mix(in srgb, var(--text) 6%, transparent);
	}

	.link.active::before {
		content: '';
		position: absolute;
		left: 0;
		top: 13px;
		bottom: 13px;
		width: 2px;
		border-radius: 1px;
		background: var(--accent);
	}

	.link.active :global(svg) {
		color: var(--accent);
	}

	.bottom {
		margin-top: auto;
		padding-top: var(--sp-6);
		display: grid;
		gap: 2px;
	}

	.collapse {
		color: var(--text-muted);
	}

	.foot {
		display: flex;
		justify-content: space-between;
		gap: var(--sp-2);
		padding: var(--sp-4) 14px 0;
		margin-top: var(--sp-3);
		border-top: 1px solid var(--line);
		color: var(--text-muted);
		letter-spacing: 0.04em;
		text-transform: none;
		font-weight: 400;
	}

	.foot a:hover {
		color: var(--text);
	}

	/* Collapsed: icons only. */
	.collapsed {
		padding-inline: var(--sp-3);
	}

	.collapsed .word,
	.collapsed .label,
	.collapsed .foot {
		opacity: 0;
		pointer-events: none;
	}

	.collapsed .section {
		opacity: 0;
		height: var(--sp-5);
		padding: 0;
	}

	.collapsed .brand {
		padding: 0 17px;
	}

	.collapsed .link {
		padding: 0 14px;
	}
</style>
