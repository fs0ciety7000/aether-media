<script lang="ts">
	import { fly, fade } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { flip } from 'svelte/animate';
	import { toasts } from '$lib/toast.svelte';
	import Icon from './Icon.svelte';

	const reduced = typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;
</script>

<section class="toaster" aria-live="polite" aria-label="Notifications">
	{#each toasts.items as toast (toast.id)}
		<div
			class="toast"
			role="status"
			animate:flip={{ duration: reduced ? 0 : 260, easing: cubicOut }}
			in:fly={{ y: reduced ? 0 : 10, duration: 260, easing: cubicOut }}
			out:fade={{ duration: 200 }}
		>
			<div class="body">
				<p class="message">{toast.message}</p>
				{#if toast.detail}<p class="detail">{toast.detail}</p>{/if}
			</div>
			{#if toast.action}
				<button
					class="action"
					type="button"
					onclick={() => {
						toast.action?.run();
						toasts.dismiss(toast.id);
					}}>{toast.action.label}</button
				>
			{/if}
			<button class="close" type="button" aria-label="Fermer la notification" onclick={() => toasts.dismiss(toast.id)}>
				<Icon name="x" size={16} />
			</button>
		</div>
	{/each}
</section>

<style>
	.toaster {
		position: fixed;
		z-index: 60;
		right: var(--gutter);
		bottom: var(--sp-5);
		display: flex;
		flex-direction: column;
		align-items: flex-end;
		gap: var(--sp-3);
		width: min(380px, calc(100vw - 2 * var(--gutter)));
		pointer-events: none;
	}

	@media (max-width: 767px) {
		.toaster {
			bottom: calc(var(--tabbar-h) + env(safe-area-inset-bottom, 0px) + var(--sp-3));
		}
	}

	.toast {
		pointer-events: auto;
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		width: 100%;
		padding: var(--sp-3) var(--sp-3) var(--sp-3) var(--sp-4);
		background: var(--surface-raised);
		border-radius: var(--r-md);
		box-shadow: var(--shadow-3), inset 0 0 0 1px var(--line);
	}

	.body {
		flex: 1;
		min-width: 0;
	}

	.message {
		font-size: var(--fs-small);
		line-height: var(--lh-small);
		color: var(--text);
	}

	.detail {
		font-size: var(--fs-micro);
		line-height: var(--lh-micro);
		color: var(--text-muted);
		margin-top: 2px;
	}

	.action {
		flex: none;
		height: 32px;
		padding: 0 var(--sp-2);
		font-size: var(--fs-small);
		font-weight: 500;
		color: var(--accent-ink);
		border-radius: var(--r-sm);
	}

	.action:hover {
		text-decoration: underline;
		text-underline-offset: 3px;
	}

	.close {
		flex: none;
		display: grid;
		place-items: center;
		width: 32px;
		height: 32px;
		border-radius: var(--r-sm);
		color: var(--text-muted);
		transition: color var(--dur-fast) var(--ease);
	}

	.close:hover {
		color: var(--text);
	}
</style>
