<script lang="ts">
	// Small non-interactive label: genre tags and badges on artwork, status pills elsewhere.
	import type { Snippet } from 'svelte';
	import Icon, { type IconName } from './Icon.svelte';

	interface Props {
		/** image = dark matte over artwork · surface = quiet tile · accent = tinted. */
		variant?: 'image' | 'surface' | 'accent';
		icon?: IconName;
		children: Snippet;
	}

	let { variant = 'surface', icon, children }: Props = $props();
</script>

<span class={['tag', variant]}>
	{#if icon}<Icon name={icon} size={13} />{/if}
	{@render children()}
</span>

<style>
	.tag {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		height: 26px;
		padding: 0 11px;
		border-radius: var(--r-full);
		font-size: var(--fs-micro);
		font-weight: var(--fw-medium);
		line-height: 1;
		white-space: nowrap;
	}

	.surface {
		background: var(--surface-raised);
		color: var(--text-muted);
		box-shadow: inset 0 0 0 1px var(--line);
	}

	.image {
		background: rgba(4, 14, 22, 0.58);
		color: var(--on-image);
		box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.1);
	}

	.accent {
		background: var(--accent-soft);
		color: var(--accent-ink);
	}
</style>
