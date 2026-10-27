<script lang="ts">
	interface Props {
		/** 0–1 */
		value: number;
		/** overlay = flush along the bottom edge of artwork · inline = rounded bar in the flow. */
		variant?: 'overlay' | 'inline';
	}

	let { value, variant = 'overlay' }: Props = $props();
	const pct = $derived(Math.round(Math.min(1, Math.max(0, value)) * 100));
</script>

<div
	class={['progress', variant]}
	role="progressbar"
	aria-valuemin={0}
	aria-valuemax={100}
	aria-valuenow={pct}
	aria-label="Visionné à {pct} %"
>
	<span style:transform="scaleX({pct / 100})"></span>
</div>

<style>
	.progress {
		overflow: hidden;
	}

	.overlay {
		position: absolute;
		left: 0;
		right: 0;
		bottom: 0;
		height: 3px;
		background: rgba(4, 14, 22, 0.55);
	}

	.inline {
		position: relative;
		height: 3px;
		border-radius: var(--r-full);
		background: rgba(246, 255, 255, 0.2);
	}

	span {
		display: block;
		height: 100%;
		border-radius: inherit;
		background: var(--accent);
		transform-origin: left;
	}
</style>
