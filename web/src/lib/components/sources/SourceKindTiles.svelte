<script lang="ts">
	// The three source types as large tiles. Buttons: Tab / arrow keys move, Enter or Space picks.
	import type { SourceKind } from '$lib/api/types';
	import { SOURCE_KINDS } from '$lib/sources';
	import Icon from '../Icon.svelte';

	interface Props {
		onpick: (kind: SourceKind) => void;
		/** Forces a visual state in the design gallery. */
		demo?: SourceKind;
		label?: string;
	}

	let { onpick, demo, label = 'Type de source' }: Props = $props();

	let list = $state<HTMLElement>();

	function onkeydown(e: KeyboardEvent) {
		const keys = ['ArrowDown', 'ArrowRight', 'ArrowUp', 'ArrowLeft', 'Home', 'End'];
		if (!keys.includes(e.key) || !list) return;
		const tiles = [...list.querySelectorAll<HTMLButtonElement>('button.tile')];
		const i = tiles.indexOf(document.activeElement as HTMLButtonElement);
		if (i < 0) return;
		e.preventDefault();
		const n = tiles.length;
		const next = e.key === 'Home' ? 0 : e.key === 'End' ? n - 1 : e.key === 'ArrowDown' || e.key === 'ArrowRight' ? (i + 1) % n : (i - 1 + n) % n;
		tiles[next].focus();
	}
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<ul class="tiles" role="list" aria-label={label} bind:this={list} {onkeydown}>
	{#each SOURCE_KINDS as k (k.id)}
		<li>
			<button type="button" class="tile" data-kind={k.id} data-demo={demo === k.id ? 'hover' : undefined} onclick={() => onpick(k.id)}>
				<span class="t-icon" aria-hidden="true"><Icon name={k.icon} size={24} /></span>
				<span class="t-text">
					<strong>{k.label}</strong>
					<span>{k.hint}</span>
				</span>
				<span class="t-go" aria-hidden="true"><Icon name="chevron-right" size={18} /></span>
			</button>
		</li>
	{/each}
</ul>

<style>
	.tiles {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: var(--sp-3);
	}

	.tile {
		position: relative;
		display: grid;
		grid-template-rows: auto 1fr;
		align-content: start;
		gap: var(--sp-3);
		width: 100%;
		height: 100%;
		min-height: 168px;
		padding: var(--sp-4);
		border-radius: var(--r-lg);
		background: var(--tint), var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
		color: var(--text-muted);
		text-align: left;
		transition:
			background-color var(--dur-fast) var(--ease),
			box-shadow var(--dur-fast) var(--ease),
			transform var(--dur-fast) var(--ease);
	}

	.tile:hover,
	.tile[data-demo='hover'] {
		background: var(--tint), var(--surface-active);
		box-shadow: inset 0 0 0 1.5px color-mix(in srgb, var(--accent) 55%, transparent);
	}

	.tile:active {
		transform: translateY(1px);
	}

	.t-icon {
		display: grid;
		place-items: center;
		width: 46px;
		height: 46px;
		border-radius: 14px;
		background: var(--accent-soft);
		color: var(--accent-ink);
	}

	.t-text {
		display: grid;
		gap: 4px;
		align-content: start;
		font-size: var(--fs-small);
		line-height: var(--lh-small);
	}

	.t-text strong {
		font-size: var(--fs-h3, 1rem);
		color: var(--text);
	}

	.t-go {
		position: absolute;
		top: var(--sp-4);
		right: var(--sp-4);
		color: var(--text-faint);
		transition:
			color var(--dur-fast) var(--ease),
			transform var(--dur-fast) var(--ease);
	}

	.tile:hover .t-go,
	.tile:focus-visible .t-go,
	.tile[data-demo='hover'] .t-go {
		color: var(--accent-ink);
		transform: translateX(2px);
	}

	@media (max-width: 767px) {
		.tiles {
			grid-template-columns: minmax(0, 1fr);
			gap: var(--sp-2);
		}

		.tile {
			grid-template-columns: auto minmax(0, 1fr);
			grid-template-rows: none;
			align-items: center;
			min-height: 0;
			padding: var(--sp-3) var(--sp-8) var(--sp-3) var(--sp-3);
		}

		.t-go {
			top: 50%;
			margin-top: -9px;
		}
	}
</style>
