<script lang="ts">
	// Section title (20 px semibold) with an optional muted "Tout voir" link on the right.
	import type { Snippet } from 'svelte';

	interface Props {
		title: string;
		id?: string;
		href?: string;
		linkLabel?: string;
		/** Heading level (default h2). */
		level?: 2 | 3;
		/** Extra controls next to the link (e.g. carousel arrows). */
		actions?: Snippet;
	}

	let { title, id, href, linkLabel = 'Tout voir', level = 2, actions }: Props = $props();
</script>

<header class="section-header">
	<svelte:element this={`h${level}`} {id} class="title">{title}</svelte:element>
	<div class="end">
		{#if actions}{@render actions()}{/if}
		{#if href}
			<a class="see-all" {href} aria-label="{linkLabel} — {title}">{linkLabel}</a>
		{/if}
	</div>
</header>

<style>
	.section-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--sp-4);
		min-height: 32px;
		margin-bottom: var(--sp-3);
	}

	.title {
		font-size: var(--fs-h2);
		line-height: var(--lh-h2);
		font-weight: var(--fw-bold);
		letter-spacing: -0.005em;
		color: var(--text);
	}

	.end {
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		flex: none;
	}

	.see-all {
		font-size: var(--fs-small);
		font-weight: var(--fw-medium);
		color: var(--text-muted);
		border-radius: var(--r-xs);
		transition: color var(--dur-fast) var(--ease);
	}

	.see-all:hover {
		color: var(--text);
	}
</style>
