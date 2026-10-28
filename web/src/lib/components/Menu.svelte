<script lang="ts" module>
	import type { IconName } from './Icon.svelte';

	export interface MenuItem {
		label: string;
		icon?: IconName;
		onselect: () => void;
		disabled?: boolean;
		/** Destructive / leaving action (sign out): drawn after a separator. */
		separated?: boolean;
	}
</script>

<script lang="ts">
	// Action menu (menu button pattern): ↑ ↓ Home End to move, Enter/Space to run, Esc to close.
	import type { Snippet } from 'svelte';
	import Icon from './Icon.svelte';

	interface Props {
		label: string;
		/** The trigger snippet shows its own text (WCAG 2.5.3 label in name): the button takes
		 *  its name from that content, `label` only names the menu. */
		labelFromContent?: boolean;
		items: MenuItem[];
		/** Trigger icon. */
		icon?: IconName;
		/** Visible trigger text (icon-only when omitted). */
		text?: string;
		align?: 'start' | 'end';
		/** Trigger style (bare: the `trigger` snippet draws everything). */
		variant?: 'secondary' | 'image' | 'bare';
		/** Custom trigger content (user block). */
		trigger?: Snippet;
		/** Non-interactive block at the top of the list (signed-in user). */
		header?: Snippet;
	}

	let { label, labelFromContent = false, items, icon = 'more', text, align = 'end', variant = 'secondary', trigger: triggerContent, header }: Props = $props();

	const uid = $props.id();
	let open = $state(false);
	let root = $state<HTMLElement>();
	let trigger = $state<HTMLButtonElement>();
	let list = $state<HTMLElement>();

	function buttons() {
		return Array.from(list?.querySelectorAll<HTMLButtonElement>('[role=menuitem]:not([disabled])') ?? []);
	}

	function show(focus: 'first' | 'last' = 'first') {
		open = true;
		requestAnimationFrame(() => {
			const b = buttons();
			(focus === 'first' ? b[0] : b[b.length - 1])?.focus();
		});
	}

	function hide(returnFocus = true) {
		open = false;
		if (returnFocus) trigger?.focus();
	}

	function onTriggerKey(e: KeyboardEvent) {
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			show('first');
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			show('last');
		}
	}

	function onListKey(e: KeyboardEvent) {
		const b = buttons();
		const i = b.indexOf(document.activeElement as HTMLButtonElement);
		if (e.key === 'ArrowDown') b[(i + 1) % b.length]?.focus();
		else if (e.key === 'ArrowUp') b[(i - 1 + b.length) % b.length]?.focus();
		else if (e.key === 'Home') b[0]?.focus();
		else if (e.key === 'End') b[b.length - 1]?.focus();
		else if (e.key === 'Escape') hide();
		else if (e.key === 'Tab') hide(false);
		else return;
		e.preventDefault();
	}

	function run(item: MenuItem) {
		hide(false);
		trigger?.focus();
		item.onselect();
	}

	function onWindowPointer(e: PointerEvent) {
		if (open && root && !root.contains(e.target as Node)) open = false;
	}
</script>

<svelte:window onpointerdown={onWindowPointer} />

<div class="menu" bind:this={root}>
	<button
		bind:this={trigger}
		type="button"
		class="trigger {variant}"
		class:text={!!text}
		aria-haspopup="menu"
		aria-expanded={open}
		aria-controls="menu-{uid}"
		aria-label={text || labelFromContent ? undefined : label}
		onclick={() => (open ? hide() : show())}
		onkeydown={onTriggerKey}
	>
		{#if triggerContent}
			{@render triggerContent()}
		{:else}
			<Icon name={icon} size={20} />
			{#if text}<span>{text}</span>{/if}
		{/if}
	</button>

	{#if open}
		<!-- svelte-ignore a11y_interactive_supports_focus -->
		<div class="list {align}" id="menu-{uid}" role="menu" aria-label={label} bind:this={list} onkeydown={onListKey}>
 			{#if header}<div class="header" role="presentation">{@render header()}</div>{/if}
			{#each items as item (item.label)}
				{#if item.separated}<div class="sep" role="separator"></div>{/if}
				<button type="button" role="menuitem" tabindex="-1" disabled={item.disabled} onclick={() => run(item)}>
					{#if item.icon}<Icon name={item.icon} size={18} />{/if}
					<span>{item.label}</span>
				</button>
			{/each}
		</div>
	{/if}
</div>

<style>
	.menu {
		position: relative;
		display: inline-flex;
	}

	.trigger {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: var(--sp-2);
		width: 46px;
		height: 46px;
		border-radius: var(--r-full);
		background: var(--surface-raised);
		color: var(--text);
		box-shadow: inset 0 0 0 1px var(--line-strong);
		font-weight: var(--fw-bold);
		transition: background-color var(--dur-fast) var(--ease);
	}

	.trigger.bare {
		width: auto;
		height: auto;
		background: none;
		box-shadow: none;
		border-radius: var(--r-full);
		font-weight: inherit;
	}

	.trigger.bare:hover,
	.trigger.bare[aria-expanded='true'] {
		background: color-mix(in srgb, var(--text) 5%, transparent);
	}

	.header {
		padding: var(--sp-3) var(--sp-3) var(--sp-3);
		margin-bottom: 4px;
		box-shadow: 0 1px 0 var(--line);
	}

	.sep {
		height: 1px;
		margin: 4px 6px;
		background: var(--line);
	}

	.trigger.text {
		width: auto;
		padding: 0 var(--sp-5) 0 var(--sp-4);
	}

	.trigger.image {
		background: rgba(4, 14, 22, 0.6);
		color: var(--on-image);
		box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.14);
	}

	.trigger:hover,
	.trigger[aria-expanded='true'] {
		background: color-mix(in srgb, var(--surface-raised) 86%, var(--text));
	}

	.trigger.image:hover,
	.trigger.image[aria-expanded='true'] {
		background: rgba(4, 14, 22, 0.8);
	}

	.list {
		position: absolute;
		top: calc(100% + var(--sp-2));
		z-index: 40;
		min-width: 236px;
		padding: 6px;
		background: var(--tint), var(--surface-raised);
		border-radius: var(--r-lg);
		box-shadow: var(--shadow-3), inset 0 0 0 1px var(--line);
		color: var(--text);
		animation: appear var(--dur-base) var(--ease);
	}

	.list.end {
		right: 0;
	}

	.list.start {
		left: 0;
	}

	@keyframes appear {
		from {
			opacity: 0;
			transform: translateY(-4px);
		}
	}

	[role='menuitem'] {
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		width: 100%;
		height: 40px;
		padding: 0 var(--sp-3);
		border-radius: var(--r-sm);
		font-size: var(--fs-body);
		font-weight: var(--fw-medium);
		color: var(--text);
		text-align: left;
		white-space: nowrap;
	}

	[role='menuitem'] :global(svg) {
		color: var(--text-muted);
	}

	[role='menuitem']:hover:not(:disabled),
	[role='menuitem']:focus-visible {
		background: color-mix(in srgb, var(--text) 7%, transparent);
		outline: none;
	}

	[role='menuitem']:disabled {
		opacity: 0.45;
	}
</style>
