<script lang="ts">
	// Modal dialog on the native <dialog>: the page behind is inert (focus trap), Esc closes,
	// focus goes to the first field (or `initialFocus`) and returns to the opener on close.
	import type { Snippet } from 'svelte';
	import Icon from './Icon.svelte';

	interface Props {
		open: boolean;
		title: string;
		description?: string;
		size?: 'sm' | 'md' | 'lg';
		children: Snippet;
		footer?: Snippet;
		/** CSS selector of the element to focus first. */
		initialFocus?: string;
		onclose?: () => void;
	}

	let { open = $bindable(false), title, description, size = 'md', children, footer, initialFocus, onclose }: Props = $props();

	const uid = $props.id();
	let dialog = $state<HTMLDialogElement>();
	let opener: HTMLElement | null = null;

	$effect(() => {
		if (!dialog) return;
		if (open && !dialog.open) {
			opener = document.activeElement as HTMLElement | null;
			dialog.showModal();
			queueMicrotask(() => {
				const target =
					(initialFocus && dialog?.querySelector<HTMLElement>(initialFocus)) ||
					dialog?.querySelector<HTMLElement>('.body input:not([type=hidden]), .body select, .body textarea, .body button, .foot button');
				target?.focus();
			});
		} else if (!open && dialog.open) {
			dialog.close();
		}
	});

	function onDialogClose() {
		open = false;
		onclose?.();
		// Restore focus to whatever opened the dialog (menus may have unmounted: fall back to main).
		const back = opener && opener.isConnected ? opener : document.getElementById('contenu');
		back?.focus({ preventScroll: true });
		opener = null;
	}

	function onBackdrop(e: MouseEvent) {
		if (e.target === dialog) open = false;
	}
</script>

<dialog
	bind:this={dialog}
	class="dialog {size}"
	aria-labelledby="dlg-{uid}-title"
	aria-describedby={description ? `dlg-${uid}-desc` : undefined}
	onclose={onDialogClose}
	onclick={onBackdrop}
>
	{#if open}
		<div class="sheet">
			<header class="head">
				<div class="titles">
					<h2 id="dlg-{uid}-title">{title}</h2>
					{#if description}<p class="desc" id="dlg-{uid}-desc">{description}</p>{/if}
				</div>
				<button class="close" type="button" aria-label="Fermer" onclick={() => (open = false)}>
					<Icon name="x" size={18} />
				</button>
			</header>
			<div class="body">{@render children()}</div>
			{#if footer}<footer class="foot">{@render footer()}</footer>{/if}
		</div>
	{/if}
</dialog>

<style>
	.dialog {
		width: min(560px, calc(100vw - 2 * var(--sp-4)));
		max-height: min(86vh, 900px);
		margin: auto;
		padding: 0;
		border: 0;
		border-radius: var(--r-xl);
		background: var(--tint), var(--surface-raised);
		color: var(--text);
		box-shadow: var(--shadow-3), inset 0 0 0 1px var(--line);
		overflow: hidden;
	}

	.sm {
		width: min(420px, calc(100vw - 2 * var(--sp-4)));
	}

	.lg {
		width: min(880px, calc(100vw - 2 * var(--sp-4)));
	}

	.dialog::backdrop {
		background: rgba(2, 8, 13, 0.62);
	}

	.dialog[open] {
		animation: appear var(--dur-base) var(--ease);
	}

	.dialog[open]::backdrop {
		animation: fade var(--dur-base) var(--ease);
	}

	@keyframes appear {
		from {
			opacity: 0;
			transform: translateY(8px) scale(0.99);
		}
	}

	@keyframes fade {
		from {
			opacity: 0;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		@keyframes appear {
			from {
				opacity: 0;
			}
		}
	}

	.sheet {
		display: flex;
		flex-direction: column;
		max-height: inherit;
	}

	.head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: var(--sp-4);
		padding: var(--sp-5) var(--sp-5) var(--sp-4);
	}

	.titles {
		display: grid;
		gap: var(--sp-1);
		min-width: 0;
	}

	.desc {
		font-size: var(--fs-small);
		color: var(--text-muted);
	}

	.close {
		display: grid;
		place-items: center;
		flex: none;
		width: 36px;
		height: 36px;
		margin: -6px -8px 0 0;
		border-radius: var(--r-sm);
		color: var(--text-muted);
		transition:
			color var(--dur-fast) var(--ease),
			background-color var(--dur-fast) var(--ease);
	}

	.close:hover {
		color: var(--text);
		background: color-mix(in srgb, var(--text) 7%, transparent);
	}

	.body {
		padding: 0 var(--sp-5) var(--sp-5);
		overflow-y: auto;
		overscroll-behavior: contain;
	}

	.foot {
		display: flex;
		justify-content: flex-end;
		flex-wrap: wrap;
		gap: var(--sp-3);
		padding: var(--sp-4) var(--sp-5);
		box-shadow: 0 -1px 0 var(--line);
	}

	@media (max-width: 767px) {
		.dialog,
		.sm,
		.lg {
			width: 100vw;
			max-width: none;
			max-height: 92dvh;
			margin: auto 0 0;
			border-radius: var(--r-xl) var(--r-xl) 0 0;
		}

		.head {
			padding: var(--sp-4) var(--sp-4) var(--sp-3);
		}

		.body {
			padding: 0 var(--sp-4) var(--sp-4);
		}

		.foot {
			padding: var(--sp-3) var(--sp-4) calc(var(--sp-3) + env(safe-area-inset-bottom, 0px));
		}

		.foot :global(.btn) {
			flex: 1;
		}
	}
</style>
