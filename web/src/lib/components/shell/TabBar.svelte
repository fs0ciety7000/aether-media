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
					<Icon name={item.icon} size={21} filled={active && item.icon === 'home'} />
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
		padding: 0 var(--sp-2) env(safe-area-inset-bottom, 0px);
		background: var(--surface);
		box-shadow: 0 -1px 0 var(--line), 0 -12px 28px rgba(0, 0, 0, 0.22);
	}

	ul {
		display: grid;
		grid-template-columns: repeat(5, 1fr);
		height: var(--tabbar-h);
		align-items: center;
	}

	a {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 3px;
		height: 54px;
		margin: 0 3px;
		border-radius: var(--r-md);
		color: var(--text-muted);
		font-size: 0.6875rem;
		font-weight: var(--fw-medium);
		transition:
			color var(--dur-fast) var(--ease),
			background-color var(--dur-fast) var(--ease);
	}

	a:focus-visible {
		outline-offset: -2px;
	}

	a.active {
		color: var(--text);
		background: var(--surface-active);
		font-weight: var(--fw-bold);
	}

	a.active :global(svg) {
		color: var(--accent);
	}
</style>
