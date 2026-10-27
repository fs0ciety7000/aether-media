<script lang="ts">
	// Generated 16:9 "film still": a quiet landscape in muted tones of one hue.
	import type { BackdropScene } from './data';

	interface Props {
		hue: number;
		sat?: number;
		scene?: BackdropScene;
	}

	let { hue, sat = 18, scene = 'dunes' }: Props = $props();

	const uid = $props.id();
	const c = (s: number, l: number, a = 1) => `hsl(${hue} ${Math.min(24, s)}% ${l}% / ${a})`;
</script>

<svg class="art" viewBox="0 0 1600 900" preserveAspectRatio="xMidYMid slice" aria-hidden="true">
	<defs>
		<linearGradient id="sky-{uid}" x1="0" y1="0" x2="0" y2="1">
			<stop offset="0" stop-color={c(sat, 11)} />
			<stop offset="0.6" stop-color={c(sat + 2, 30)} />
			<stop offset="1" stop-color={c(sat + 6, 52)} />
		</linearGradient>
		<radialGradient id="glow-{uid}" cx="0.5" cy="0.5" r="0.5">
			<stop offset="0" stop-color={c(sat + 6, 72, 0.6)} />
			<stop offset="0.35" stop-color={c(sat + 4, 56, 0.25)} />
			<stop offset="1" stop-color={c(sat, 40, 0)} />
		</radialGradient>
		<radialGradient id="disc-{uid}" cx="0.45" cy="0.4" r="0.6">
			<stop offset="0" stop-color={c(sat + 8, 86)} />
			<stop offset="0.8" stop-color={c(sat + 6, 72)} />
			<stop offset="1" stop-color={c(sat + 6, 66, 0.9)} />
		</radialGradient>
		<linearGradient id="ground-{uid}" x1="0" y1="0" x2="0" y2="1">
			<stop offset="0" stop-color={c(sat, 20)} />
			<stop offset="1" stop-color={c(sat, 7)} />
		</linearGradient>
	</defs>

	{#if scene === 'room'}
		<rect width="1600" height="900" fill={c(sat, 10)} />
		<rect x="980" y="150" width="300" height="470" fill={c(sat + 4, 52, 0.75)} />
		<path d="M980 150h300v470H980z M1130 150v470 M980 385h300" fill="none" stroke={c(sat, 14)} stroke-width="10" />
		<polygon points="980,620 1280,620 1500,900 700,900" fill={c(sat + 4, 40, 0.22)} />
		<rect x="0" y="620" width="1600" height="280" fill={c(sat, 8, 0.6)} />
		<rect x="900" y="560" width="70" height="200" fill={c(sat, 6)} />
	{:else}
		<rect width="1600" height="900" fill="url(#sky-{uid})" />
		<circle cx="1120" cy={scene === 'sea' ? 470 : 430} r="420" fill="url(#glow-{uid})" />
		<circle cx="1120" cy={scene === 'sea' ? 470 : 430} r="70" fill="url(#disc-{uid})" opacity="0.85" />
		{#if scene === 'dunes'}
			<path d="M0 640 C 300 560 520 600 760 650 S 1260 560 1600 610 V900 H0Z" fill={c(sat + 4, 30)} />
			<path d="M0 730 C 380 650 700 720 980 760 S 1400 690 1600 720 V900 H0Z" fill={c(sat + 2, 19)} />
			<path d="M0 820 C 420 770 900 820 1600 800 V900 H0Z" fill={c(sat, 8)} />
		{:else if scene === 'sea'}
			<rect x="0" y="560" width="1600" height="340" fill="url(#ground-{uid})" />
			<rect x="1084" y="562" width="72" height="300" fill={c(sat + 6, 70, 0.18)} />
			<rect x="0" y="559" width="1600" height="2" fill={c(sat, 72, 0.35)} />
		{:else}
			<path d="M0 600 L220 470 L380 540 L620 380 L840 520 L1000 450 L1240 560 L1420 470 L1600 540 V900H0Z" fill={c(sat + 2, 27)} />
			<path d="M0 700 L260 600 L520 680 L760 560 L1060 690 L1320 610 L1600 680 V900H0Z" fill={c(sat, 14)} />
			<path d="M0 800 L400 730 L800 790 L1200 740 L1600 790 V900H0Z" fill={c(sat, 7)} />
		{/if}
	{/if}
</svg>

<style>
	.art {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
	}
</style>
