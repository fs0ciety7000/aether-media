<script lang="ts">
	// Watch state over artwork (top-right): "Vu" check, or the number of unwatched episodes.
	import Icon from './Icon.svelte';

	interface Props {
		/** Watched (movie, episode) or fully watched (series, season). */
		played?: boolean;
		/** Unwatched episodes (series, season); hidden at 0. */
		unplayed?: number;
		class?: string;
	}

	let { played = false, unplayed, class: className }: Props = $props();
</script>

{#if played}
	<span class={['watch-badge', 'played', className]} title="Vu">
		<Icon name="check" size={14} />
		<span class="visually-hidden">Vu</span>
	</span>
{:else if unplayed}
	<span class={['watch-badge', 'count', 'tabular', className]} title="{unplayed} non vu{unplayed > 1 ? 's' : ''}">
		<span aria-hidden="true">{unplayed > 99 ? '99+' : unplayed}</span>
		<span class="visually-hidden">{unplayed} épisode{unplayed > 1 ? 's' : ''} non vu{unplayed > 1 ? 's' : ''}</span>
	</span>
{/if}

<style>
	.watch-badge {
		display: inline-grid;
		place-items: center;
		height: 24px;
		border-radius: var(--r-full);
		font-size: var(--fs-micro);
		font-weight: var(--fw-heavy);
		line-height: 1;
		box-shadow: 0 2px 8px rgba(2, 10, 16, 0.35);
	}

	.played {
		width: 24px;
		background: var(--accent);
		color: var(--accent-contrast);
	}

	.played :global(svg) {
		stroke-width: 2.4;
	}

	.count {
		min-width: 24px;
		padding: 0 7px;
		background: rgba(4, 14, 22, 0.72);
		color: var(--on-image);
		box-shadow:
			inset 0 0 0 1px rgba(255, 255, 255, 0.12),
			0 2px 8px rgba(2, 10, 16, 0.35);
	}
</style>
