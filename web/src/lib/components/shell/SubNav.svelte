<script lang="ts">
	// Section tabs as a pill group (Réglages, Profil): links, the current one marked with aria-current.
	import { page } from '$app/state';
	import { isActive, type NavItem } from '$lib/nav';
	import Icon from '../Icon.svelte';

	interface Props {
		items: NavItem[];
		label: string;
		/** Entries matched exactly (a parent route such as /profil would otherwise stay active). */
		exact?: string[];
	}

	let { items, label, exact = [] }: Props = $props();

	const active = (href: string) => (exact.includes(href) ? page.url.pathname === href : isActive(page.url.pathname, href));
</script>

<nav class="sub-nav" aria-label={label}>
	<ul role="list">
		{#each items as item (item.href)}
			{@const on = active(item.match ?? item.href)}
			<li>
				<a href={item.href} class:active={on} aria-current={on ? 'page' : undefined}>
					<Icon name={item.icon} size={16} />{item.label}
				</a>
			</li>
		{/each}
	</ul>
</nav>

<style>
	.sub-nav {
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
