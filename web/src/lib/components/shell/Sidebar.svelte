<script lang="ts">
	import { page } from '$app/state';
	import { libraries as librariesStore, unmatched } from '$lib/api/stores.svelte';
	import { libraryKindInfo } from '$lib/media';
	import { collections, discover, identify, isActive, profile, settings, type NavItem } from '$lib/nav';
	import { session } from '$lib/session.svelte';
	import Logo from '../brand/Logo.svelte';
	import Icon from '../Icon.svelte';

	interface Props {
		collapsed: boolean;
		ontoggle: () => void;
		version?: string;
	}

	let { collapsed, ontoggle, version }: Props = $props();

	const libs = $derived(librariesStore.list);
	const admin = $derived(session.isAdmin);
	const showIdentify = $derived(admin && (unmatched.count > 0 || isActive(page.url.pathname, identify.href)));
</script>

{#snippet link(item: NavItem, badge?: number)}
	{@const active = isActive(page.url.pathname, item.match ?? item.href)}
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
			{#if badge}
				<span class="count tabular" aria-label="{badge} en attente">{badge > 99 ? '99+' : badge}</span>
			{/if}
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

		<p class="section micro" id="nav-libs">Bibliothèques</p>
		<ul role="list" class="libs" aria-labelledby="nav-libs">
			{#each libs as lib (lib.id)}
				{@render link({ href: `/bibliotheque/${lib.id}`, label: lib.name, icon: libraryKindInfo(lib.kind).icon })}
			{/each}
			{#if librariesStore.loaded && !libs.length && admin}
				<li>
					<a class="link add" href="/reglages/bibliotheques?ajouter=1" title={collapsed ? 'Ajouter une bibliothèque' : undefined}>
						<Icon name="plus" size={18} />
						<span class="label">Ajouter une bibliothèque</span>
					</a>
				</li>
			{/if}
			{@render link(collections)}
		</ul>
	</nav>

	<div class="bottom">
		<ul role="list">
			{#if showIdentify}{@render link(identify, unmatched.count)}{/if}
			{#if admin}{@render link(settings)}{:else}{@render link(profile)}{/if}
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

	.section {
		margin: var(--sp-6) 0 var(--sp-2);
		padding: 0 var(--sp-3);
		color: var(--text-muted); /* text-faint (3.8:1) fails AA for 12 px text */
		text-transform: uppercase;
		letter-spacing: 0.08em;
		white-space: nowrap;
		transition: opacity var(--dur-base) var(--ease);
	}

	.link.add {
		background: none;
		color: var(--text-muted);
		box-shadow: inset 0 0 0 1px var(--line);
		border: 0;
	}

	.count {
		margin-left: auto;
		min-width: 22px;
		height: 20px;
		padding: 0 6px;
		border-radius: var(--r-full);
		background: var(--accent-soft);
		color: var(--accent-ink);
		font-size: var(--fs-micro);
		font-weight: var(--fw-bold);
		line-height: 20px;
		text-align: center;
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

	.collapsed .count {
		position: absolute;
		top: 4px;
		right: 6px;
		min-width: 8px;
		width: 8px;
		height: 8px;
		padding: 0;
		font-size: 0;
		background: var(--accent);
	}

	.collapsed .link {
		position: relative;
	}

	.collapsed .section {
		opacity: 0;
		height: 0;
		margin: var(--sp-5) 0 0;
		overflow: hidden;
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
