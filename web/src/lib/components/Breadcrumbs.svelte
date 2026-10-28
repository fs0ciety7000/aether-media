<script lang="ts">
	import Icon from './Icon.svelte';

	interface Props {
		items: { label: string; href?: string }[];
		/** Light ink over artwork. */
		onImage?: boolean;
	}

	let { items, onImage = false }: Props = $props();
</script>

<nav class="crumbs" class:on-image={onImage} aria-label="Fil d’Ariane">
	<ol role="list">
		{#each items as c, i (i)}
			<li>
				{#if i > 0}<Icon name="chevron-right" size={14} />{/if}
				{#if c.href && i < items.length - 1}
					<a href={c.href}>{c.label}</a>
				{:else}
					<span aria-current={i === items.length - 1 ? 'page' : undefined}>{c.label}</span>
				{/if}
			</li>
		{/each}
	</ol>
</nav>

<style>
	ol {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 2px var(--sp-1);
		font-size: var(--fs-small);
		font-weight: var(--fw-medium);
		color: var(--text-muted);
	}

	li {
		display: inline-flex;
		align-items: center;
		gap: var(--sp-1);
		min-width: 0;
	}

	a {
		border-radius: var(--r-xs);
		transition: color var(--dur-fast) var(--ease);
	}

	a:hover {
		color: var(--text);
		text-decoration: underline;
		text-underline-offset: 3px;
	}

	[aria-current] {
		color: var(--text);
	}

	.on-image {
		--text: var(--on-image);
		--text-muted: var(--on-image-muted);
	}
</style>
