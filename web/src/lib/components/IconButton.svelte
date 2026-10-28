<script lang="ts">
	// Square rounded tile holding a single icon (bell, filters, theme…). Always needs an aria-label.
	import type { HTMLAnchorAttributes, HTMLButtonAttributes } from 'svelte/elements';
	import Icon, { type IconName } from './Icon.svelte';

	type Common = {
		icon: IconName;
		/** surface = quiet tile · accent = filled (active) · ghost = no ground until hover. */
		variant?: 'surface' | 'accent' | 'ghost';
		/** sm = 36 px, md = 40 px, lg = 48 px */
		size?: 'sm' | 'md' | 'lg';
		/** Small dot in the corner (unread notifications). */
		badge?: boolean;
		filled?: boolean;
		'aria-label': string;
		'data-demo'?: string;
	};
	type Props = Common &
		((Omit<HTMLButtonAttributes, 'aria-label'> & { href?: undefined }) | (Omit<HTMLAnchorAttributes, 'aria-label'> & { href: string }));

	let { icon, variant = 'surface', size = 'md', badge = false, filled = false, class: className, ...rest }: Props = $props();

	const iconSize = $derived(size === 'lg' ? 22 : size === 'sm' ? 18 : 20);
</script>

{#if rest.href !== undefined}
	<a class={['icon-btn', variant, size, className]} {...rest as HTMLAnchorAttributes}>
		<Icon name={icon} size={iconSize} {filled} />
		{#if badge}<span class="badge" aria-hidden="true"></span>{/if}
	</a>
{:else}
	<button class={['icon-btn', variant, size, className]} type="button" {...rest as HTMLButtonAttributes}>
		<Icon name={icon} size={iconSize} {filled} />
		{#if badge}<span class="badge" aria-hidden="true"></span>{/if}
	</button>
{/if}

<style>
	.icon-btn {
		--s: 40px;
		position: relative;
		display: inline-grid;
		place-items: center;
		flex: none;
		width: var(--s);
		height: var(--s);
		border-radius: var(--r-md);
		color: var(--text-muted);
		transition:
			background-color var(--dur-fast) var(--ease),
			color var(--dur-fast) var(--ease),
			box-shadow var(--dur-fast) var(--ease);
	}

	.sm {
		--s: 36px;
		border-radius: var(--r-sm);
	}

	.lg {
		--s: 48px;
		border-radius: 14px;
	}

	.surface {
		background: var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
	}

	.surface:hover:not(:disabled):not([aria-disabled='true']),
	.surface[data-demo='hover'],
	.surface[aria-expanded='true'] {
		color: var(--text);
		background: var(--surface-raised);
		box-shadow: inset 0 0 0 1px var(--line-strong);
	}

	.accent {
		background: var(--accent);
		color: var(--accent-contrast);
	}

	.accent:hover:not(:disabled),
	.accent[data-demo='hover'] {
		background: color-mix(in srgb, var(--accent) 88%, var(--text));
	}

	.ghost:hover:not(:disabled),
	.ghost[data-demo='hover'],
	.ghost[aria-expanded='true'] {
		color: var(--text);
		background: color-mix(in srgb, var(--text) 7%, transparent);
	}

	.icon-btn:disabled,
	.icon-btn[aria-disabled='true'] {
		opacity: 0.4;
		cursor: default;
	}

	.badge {
		position: absolute;
		top: 9px;
		right: 10px;
		width: 7px;
		height: 7px;
		border-radius: 50%;
		background: var(--accent);
		box-shadow: 0 0 0 2px var(--surface);
	}

	.sm .badge {
		top: 8px;
		right: 8px;
	}
</style>
