<script lang="ts">
	// Calm empty / error state inside a tinted panel.
	import type { Snippet } from 'svelte';
	import Icon, { type IconName } from './Icon.svelte';

	interface Props {
		icon?: IconName;
		title: string;
		children?: Snippet;
		actions?: Snippet;
		/** Live region for errors and "no result" messages. */
		role?: 'status' | 'alert';
		compact?: boolean;
	}

	let { icon = 'info', title, children, actions, role, compact = false }: Props = $props();
</script>

<div class="empty panel" class:compact {role}>
	<span class="icon" aria-hidden="true"><Icon name={icon} size={compact ? 20 : 24} /></span>
	<h2 class="title">{title}</h2>
	{#if children}<div class="text">{@render children()}</div>{/if}
	{#if actions}<div class="actions">{@render actions()}</div>{/if}
</div>

<style>
	.empty {
		display: grid;
		justify-items: center;
		gap: var(--sp-3);
		padding: var(--sp-8) var(--sp-5);
		text-align: center;
	}

	.compact {
		padding: var(--sp-6) var(--sp-4);
	}

	.icon {
		display: grid;
		place-items: center;
		width: 52px;
		height: 52px;
		border-radius: 14px;
		background: var(--accent-soft);
		color: var(--accent-ink);
	}

	.compact .icon {
		width: 42px;
		height: 42px;
		border-radius: var(--r-md);
	}

	.title {
		font-size: var(--fs-h2);
	}

	.compact .title {
		font-size: var(--fs-h3);
	}

	.text {
		max-width: 52ch;
		color: var(--text-muted);
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		justify-content: center;
		gap: var(--sp-3);
		margin-top: var(--sp-2);
	}
</style>
