<script lang="ts">
	interface Props {
		/** 0–1 */
		value: number;
	}

	let { value }: Props = $props();
	const pct = $derived(Math.round(Math.min(1, Math.max(0, value)) * 100));
</script>

<div class="progress" role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={pct} aria-label="Visionné à {pct} %">
	<span style:transform="scaleX({pct / 100})"></span>
</div>

<style>
	.progress {
		position: absolute;
		left: 0;
		right: 0;
		bottom: 0;
		height: 2px;
		background: rgba(12, 11, 10, 0.55);
	}

	span {
		display: block;
		height: 100%;
		background: var(--accent);
		transform-origin: left;
	}
</style>
