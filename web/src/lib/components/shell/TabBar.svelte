<script lang="ts">
	import { page } from '$app/state';
	import { isActive, tabs } from '$lib/nav';
	import Icon from '../Icon.svelte';
</script>

<nav class="tabbar" aria-label="Navigation principale">
	<ul role="list">
		{#each tabs as item (item.href)}
			{@const active = isActive(page.url.pathname, item.href)}
			<li>
				<a href={item.href} class:active aria-current={active ? 'page' : undefined}>
					<Icon name={item.icon} size={22} />
					<span>{item.label}</span>
				</a>
			</li>
		{/each}
	</ul>
</nav>

<style>
	.tabbar {
		position: fixed;
		z-index: 30;
		left: 0;
		right: 0;
		bottom: 0;
		padding-bottom: env(safe-area-inset-bottom, 0px);
		background: var(--surface);
		box-shadow: 0 -1px 0 var(--line), 0 -8px 24px rgba(0, 0, 0, 0.12);
	}

	ul {
		display: grid;
		grid-template-columns: repeat(5, 1fr);
		height: var(--tabbar-h);
	}

	a {
		position: relative;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 4px;
		height: 100%;
		color: var(--text-muted);
		font-size: 0.6875rem;
		font-weight: 500;
		letter-spacing: 0.02em;
		transition: color var(--dur-fast) var(--ease);
	}

	a:focus-visible {
		outline-offset: -4px;
	}

	a.active {
		color: var(--text);
	}

	a.active :global(svg) {
		color: var(--accent);
	}

	a.active::before {
		content: '';
		position: absolute;
		top: 0;
		left: calc(50% - 10px);
		width: 20px;
		height: 2px;
		border-radius: 0 0 1px 1px;
		background: var(--accent);
	}
</style>
