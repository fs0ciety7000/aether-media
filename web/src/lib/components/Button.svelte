<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAnchorAttributes, HTMLButtonAttributes } from 'svelte/elements';
	import Icon, { type IconName } from './Icon.svelte';

	type Common = {
		variant?: 'primary' | 'secondary' | 'tertiary';
		/** md = 40 px, lg = 48 px */
		size?: 'md' | 'lg';
		/** Icon name, or a snippet for a custom leading visual. */
		icon?: IconName | Snippet;
		iconFilled?: boolean;
		/** Round icon-only button (requires aria-label). */
		round?: boolean;
		children?: Snippet;
	};
	type Props = Common &
		((HTMLButtonAttributes & { href?: undefined }) | (HTMLAnchorAttributes & { href: string }));

	let {
		variant = 'secondary',
		size = 'md',
		icon,
		iconFilled = false,
		round = false,
		children,
		class: className,
		...rest
	}: Props = $props();

	const iconOnly = $derived(!children);
	const iconSize = $derived(size === 'lg' ? 20 : 18);
</script>

{#snippet content()}
	{#if typeof icon === 'string'}
		<Icon name={icon} size={iconSize} filled={iconFilled} />
	{:else if icon}
		{@render icon()}
	{/if}
	{#if children}<span class="label">{@render children()}</span>{/if}
{/snippet}

{#if rest.href !== undefined}
	<a class={['btn', variant, size, { 'icon-only': iconOnly, round }, className]} {...rest as HTMLAnchorAttributes}>
		{@render content()}
	</a>
{:else}
	<button
		class={['btn', variant, size, { 'icon-only': iconOnly, round }, className]}
		type="button"
		{...rest as HTMLButtonAttributes}
	>
		{@render content()}
	</button>
{/if}

<style>
	.btn {
		--h: 40px;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: var(--sp-2);
		height: var(--h);
		padding: 0 var(--sp-4);
		border-radius: var(--r-md);
		font-size: var(--fs-small);
		font-weight: 500;
		letter-spacing: 0.01em;
		white-space: nowrap;
		user-select: none;
		transition:
			background-color var(--dur-fast) var(--ease),
			color var(--dur-fast) var(--ease),
			box-shadow var(--dur-fast) var(--ease),
			transform var(--dur-fast) var(--ease);
	}

	.lg {
		--h: 48px;
		padding: 0 var(--sp-5);
		font-size: var(--fs-body);
		gap: 10px;
	}

	.icon-only {
		width: var(--h);
		padding: 0;
	}

	.round {
		border-radius: var(--r-full);
	}

	.btn:active:not(:disabled) {
		transform: translateY(1px);
	}

	/* Primary — accent ground, used once per view. */
	.primary {
		background: var(--accent);
		color: var(--accent-contrast);
		box-shadow: var(--shadow-1);
	}
	.primary:hover:not(:disabled),
	.primary[data-demo='hover'] {
		background: color-mix(in srgb, var(--accent) 88%, var(--text));
	}

	/* Secondary — quiet surface. */
	.secondary {
		background: var(--surface-raised);
		color: var(--text);
		box-shadow: inset 0 0 0 1px var(--line);
	}
	.secondary:hover:not(:disabled),
	.secondary[data-demo='hover'] {
		background: color-mix(in srgb, var(--surface-raised) 90%, var(--text));
	}

	/* Tertiary — text only. */
	.tertiary {
		color: var(--text-muted);
		padding: 0 var(--sp-3);
	}
	.tertiary.icon-only {
		padding: 0;
	}
	.tertiary:hover:not(:disabled),
	.tertiary[data-demo='hover'] {
		color: var(--text);
		background: color-mix(in srgb, var(--text) 6%, transparent);
	}

	.btn:disabled {
		opacity: 0.4;
		box-shadow: none;
	}

	.label {
		display: inline-block;
		padding-top: 1px;
	}
</style>
