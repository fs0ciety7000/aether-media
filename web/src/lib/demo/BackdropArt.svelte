<script lang="ts">
	// Generated 16:9 key art: a night-side palette (hue2: navy/violet) lit by one warm or cool key light (hue).
	// Layered gradients and silhouettes — vivid but matte, no neon.
	import type { BackdropScene } from './data';

	interface Props {
		hue: number;
		hue2?: number;
		/** Overall saturation multiplier (0–70, default 50). */
		sat?: number;
		scene?: BackdropScene;
		/** Horizontal position of the key light, 0–1. */
		focus?: number;
	}

	let { hue, hue2 = 232, sat = 50, scene = 'dunes', focus = 0.68 }: Props = $props();

	const uid = $props.id();
	const k = $derived(sat / 50);
	/** Key light colour. */
	const L = (s: number, l: number, a = 1) => `hsl(${hue} ${Math.min(90, s * k)}% ${l}% / ${a})`;
	/** Night-side colour. */
	const N = (s: number, l: number, a = 1) => `hsl(${hue2} ${Math.min(80, s * k)}% ${l}% / ${a})`;
	/** Blend between night and key hues (t = 0 → night). */
	const mixHue = $derived(hue2 + (((hue - hue2 + 540) % 360) - 180) * 0.5);
	const M = (s: number, l: number, a = 1) => `hsl(${mixHue} ${Math.min(80, s * k)}% ${l}% / ${a})`;

	// Deterministic pseudo-random sequence from the hues and scene.
	const seed = $derived(hue * 7.13 + hue2 * 3.71 + scene.length * 11.3);
	const rnd = (i: number) => {
		const x = Math.sin(i * 12.9898 + seed * 78.233) * 43758.5453;
		return x - Math.floor(x);
	};

	const cx = $derived(Math.round(1600 * focus));
	const night = $derived(scene === 'city' || scene === 'aurora' || scene === 'planet' || scene === 'ridges' || scene === 'forest');

	const stars = $derived(
		night
			? Array.from({ length: 46 }, (_, i) => ({ x: rnd(i) * 1600, y: rnd(i + 99) * 420, r: 0.8 + rnd(i + 7) * 1.8, o: 0.25 + rnd(i + 3) * 0.6 }))
			: []
	);

	const buildings = $derived(
		scene === 'city'
			? Array.from({ length: 22 }, (_, i) => {
					const w = 50 + rnd(i + 20) * 70;
					const h = 140 + rnd(i + 40) * 330 * (1 - Math.abs(i - 11) / 16);
					return { x: i * 74 - 20, w, h };
				})
			: []
	);

	const windows = $derived(
		buildings.flatMap((b, i) =>
			Array.from({ length: 9 }, (_, j) => ({
				x: b.x + 10 + rnd(i * 13 + j) * (b.w - 24),
				y: 760 - b.h + 20 + rnd(i * 17 + j + 5) * (b.h - 60),
				on: rnd(i * 7 + j * 3) > 0.45
			})).filter((w) => w.on)
		)
	);

	const pines = (count: number, base: number, hMin: number, hVar: number, off: number) =>
		Array.from({ length: count }, (_, i) => {
			const x = (i / count) * 1700 - 50 + rnd(i + off) * 40;
			const h = hMin + rnd(i + off + 50) * hVar;
			const w = h * 0.34;
			return `M${x - w / 2} ${base} L${x} ${base - h} L${x + w / 2} ${base} Z`;
		}).join(' ');
</script>

<svg class="art" viewBox="0 0 1600 900" preserveAspectRatio="xMidYMid slice" aria-hidden="true">
	<defs>
		<linearGradient id="sky-{uid}" x1="0" y1="0" x2="0" y2="1">
			<stop offset="0" stop-color={N(48, 7)} />
			<stop offset="0.45" stop-color={M(42, 17)} />
			<stop offset="0.78" stop-color={L(52, 38)} />
			<stop offset="1" stop-color={L(62, 52)} />
		</linearGradient>
		<radialGradient id="glow-{uid}" cx="0.5" cy="0.5" r="0.5">
			<stop offset="0" stop-color={L(80, 74, 0.85)} />
			<stop offset="0.25" stop-color={L(70, 58, 0.45)} />
			<stop offset="0.6" stop-color={L(60, 45, 0.14)} />
			<stop offset="1" stop-color={L(60, 40, 0)} />
		</radialGradient>
		<radialGradient id="disc-{uid}" cx="0.42" cy="0.38" r="0.65">
			<stop offset="0" stop-color={L(70, 92)} />
			<stop offset="0.7" stop-color={L(78, 76)} />
			<stop offset="1" stop-color={L(70, 64)} />
		</radialGradient>
		<linearGradient id="water-{uid}" x1="0" y1="0" x2="0" y2="1">
			<stop offset="0" stop-color={M(45, 22)} />
			<stop offset="1" stop-color={N(50, 5)} />
		</linearGradient>
		<linearGradient id="aur-{uid}" x1="0" y1="0" x2="0" y2="1">
			<stop offset="0" stop-color={L(70, 62, 0)} />
			<stop offset="0.55" stop-color={L(70, 60, 0.5)} />
			<stop offset="1" stop-color={L(60, 50, 0)} />
		</linearGradient>
		<linearGradient id="aur2-{uid}" x1="0" y1="0" x2="0" y2="1">
			<stop offset="0" stop-color={N(60, 60, 0)} />
			<stop offset="0.6" stop-color={`hsl(${(hue + 110) % 360} ${55 * k}% 58% / 0.35)`} />
			<stop offset="1" stop-color={N(60, 50, 0)} />
		</linearGradient>
		<radialGradient id="planet-{uid}" cx="0.3" cy="0.28" r="0.85">
			<stop offset="0" stop-color={L(55, 66)} />
			<stop offset="0.45" stop-color={M(50, 34)} />
			<stop offset="1" stop-color={N(55, 9)} />
		</radialGradient>
		<linearGradient id="haze-{uid}" x1="0" y1="0" x2="0" y2="1">
			<stop offset="0" stop-color={L(50, 60, 0)} />
			<stop offset="1" stop-color={L(50, 60, 0.22)} />
		</linearGradient>
		<linearGradient id="vignette-{uid}" x1="0" y1="0" x2="0" y2="1">
			<stop offset="0" stop-color={N(50, 4, 0.35)} />
			<stop offset="0.3" stop-color={N(50, 4, 0)} />
			<stop offset="0.75" stop-color={N(50, 4, 0)} />
			<stop offset="1" stop-color={N(50, 4, 0.55)} />
		</linearGradient>
	</defs>

	<rect width="1600" height="900" fill="url(#sky-{uid})" />

	{#each stars as s, i (i)}
		<circle cx={s.x} cy={s.y} r={s.r} fill="#f6ffff" opacity={s.o * 0.7} />
	{/each}

	{#if scene === 'room'}
		<!-- Dark interior, a lit window throwing a slanted beam. -->
		<rect width="1600" height="900" fill={N(40, 7)} />
		<rect x={cx - 170} y="140" width="340" height="460" rx="6" fill="url(#sky-{uid})" />
		<circle cx={cx} cy="430" r="380" fill="url(#glow-{uid})" opacity="0.7" />
		<path d="M{cx - 170} 140h340v460h-340z M{cx} 140v460 M{cx - 170} 370h340" fill="none" stroke={N(40, 6)} stroke-width="14" />
		<polygon points="{cx - 170},600 {cx + 170},600 {cx + 520},900 {cx - 420},900" fill={L(70, 62, 0.16)} />
		<rect x="0" y="640" width="1600" height="260" fill={N(45, 5, 0.75)} />
		<path d="M{cx - 330} 900 v-210 q0 -40 40 -40 h70 q40 0 40 40 v210Z" fill={N(50, 3)} />
		<circle cx={cx - 255} cy="600" r="34" fill={N(50, 3)} />
	{:else if scene === 'planet'}
		<circle cx={cx} cy="330" r="420" fill="url(#glow-{uid})" opacity="0.55" />
		<circle cx={cx} cy="430" r="250" fill="url(#planet-{uid})" />
		<ellipse cx={cx} cy="440" rx="430" ry="70" fill="none" stroke={L(55, 72, 0.5)} stroke-width="10" transform="rotate(-12 {cx} 440)" />
		<ellipse cx={cx} cy="440" rx="470" ry="82" fill="none" stroke={M(45, 55, 0.3)} stroke-width="4" transform="rotate(-12 {cx} 440)" />
		<circle cx={cx - 520} cy="210" r="42" fill={M(40, 40)} />
		<circle cx={cx - 532} cy="198" r="42" fill={N(45, 10, 0.55)} />
		<path d="M0 760 C 300 700 620 740 900 780 S 1400 720 1600 750 V900 H0Z" fill={N(45, 8)} />
		<path d="M0 820 C 400 790 900 830 1600 810 V900 H0Z" fill={N(50, 4)} />
	{:else if scene === 'city'}
		<circle cx={cx} cy="430" r="460" fill="url(#glow-{uid})" opacity="0.85" />
		<circle cx={cx} cy="380" r="76" fill="url(#disc-{uid})" opacity="0.9" />
		{#each buildings as b, i (i)}
			<rect x={b.x + 30} y={700 - b.h * 0.8} width={b.w * 0.8} height={b.h * 0.8 + 60} fill={M(38, 18)} opacity="0.75" />
		{/each}
		{#each buildings as b, i (i)}
			<rect x={b.x} y={760 - b.h} width={b.w} height={b.h + 20} fill={N(45, 7)} />
		{/each}
		{#each windows as w, i (i)}
			<rect x={w.x} y={w.y} width="7" height="10" rx="1" fill={L(80, 70, 0.75)} />
		{/each}
		<rect x="0" y="760" width="1600" height="140" fill="url(#water-{uid})" />
		<rect x={cx - 40} y="770" width="80" height="120" fill={L(70, 70, 0.18)} />
		<rect x="0" y="760" width="1600" height="2" fill={L(60, 70, 0.3)} />
	{:else if scene === 'aurora'}
		<path d="M-50 420 C 200 180 420 330 700 200 S 1200 120 1700 260 L1700 560 C 1200 430 900 520 600 470 S 150 560 -50 560Z" fill="url(#aur-{uid})" />
		<path d="M-50 330 C 300 260 500 160 900 250 S 1400 330 1700 180 L1700 460 C 1300 520 1000 420 700 470 S 200 470 -50 460Z" fill="url(#aur2-{uid})" />
		<path d="M0 640 L180 520 L300 580 L470 430 L640 560 L800 470 L980 590 L1150 480 L1330 570 L1470 500 L1600 560 V900H0Z" fill={N(40, 14)} />
		<path d="M0 640 L180 520 L300 580 L470 430 L640 560 L800 470 L980 590 L1150 480 L1330 570 L1470 500 L1600 560" fill="none" stroke={L(60, 70, 0.35)} stroke-width="3" />
		<rect x="0" y="660" width="1600" height="240" fill="url(#water-{uid})" />
		<path d="M200 700 C 500 690 800 720 1100 700 S 1500 710 1600 705" stroke={L(60, 65, 0.25)} stroke-width="3" fill="none" />
		<path d="M0 820 C 420 780 900 830 1600 800 V900 H0Z" fill={N(50, 4)} />
	{:else if scene === 'forest'}
		<circle cx={cx} cy="470" r="480" fill="url(#glow-{uid})" opacity="0.9" />
		<path d={pines(26, 700, 160, 120, 1)} fill={M(35, 22)} opacity="0.8" />
		<rect x="0" y="560" width="1600" height="180" fill="url(#haze-{uid})" />
		<path d={pines(20, 790, 240, 160, 30)} fill={N(40, 11)} />
		<rect x="0" y="690" width="1600" height="210" fill={N(42, 9)} />
		<path d={pines(14, 920, 380, 200, 70)} fill={N(50, 4)} />
	{:else}
		<!-- Open landscapes: dunes, sea, ridges -->
		<circle cx={cx} cy={scene === 'sea' ? 500 : 440} r="470" fill="url(#glow-{uid})" />
		<circle cx={cx} cy={scene === 'sea' ? 500 : 440} r="82" fill="url(#disc-{uid})" opacity="0.92" />
		{#if scene === 'dunes'}
			<path d="M0 640 C 300 560 520 600 760 650 S 1260 560 1600 610 V900 H0Z" fill={M(45, 30)} />
			<path d="M0 640 C 300 560 520 600 760 650 S 1260 560 1600 610" fill="none" stroke={L(70, 70, 0.4)} stroke-width="3" />
			<path d="M0 730 C 380 650 700 720 980 760 S 1400 690 1600 720 V900 H0Z" fill={N(42, 15)} />
			<path d="M0 820 C 420 770 900 820 1600 800 V900 H0Z" fill={N(50, 5)} />
			<path d="M{cx - 360} 742 l8 -46 l8 46Z M{cx - 330} 748 l6 -34 l6 34Z" fill={N(50, 5)} />
		{:else if scene === 'sea'}
			<rect x="0" y="560" width="1600" height="340" fill="url(#water-{uid})" />
			<rect x={cx - 70} y="562" width="140" height="320" fill={L(70, 72, 0.2)} />
			<rect x={cx - 22} y="562" width="44" height="300" fill={L(70, 80, 0.22)} />
			<rect x="0" y="559" width="1600" height="2" fill={L(60, 72, 0.4)} />
			<path d="M0 640 L120 610 L260 640 L380 600 L520 650 V900 H0Z" fill={N(50, 5)} />
			<path d="M330 607 h14 v-120 h-14Z M322 487 h30 v-16 h-30Z" fill={N(50, 5)} />
			<circle cx="337" cy="479" r="5" fill={L(80, 80, 0.9)} />
		{:else}
			<path d="M0 600 L220 470 L380 540 L620 380 L840 520 L1000 450 L1240 560 L1420 470 L1600 540 V900H0Z" fill={M(40, 24)} />
			<path d="M0 600 L220 470 L380 540 L620 380 L840 520 L1000 450 L1240 560 L1420 470 L1600 540" fill="none" stroke={L(60, 72, 0.4)} stroke-width="3" />
			<rect x="0" y="520" width="1600" height="200" fill="url(#haze-{uid})" />
			<path d="M0 700 L260 600 L520 680 L760 560 L1060 690 L1320 610 L1600 680 V900H0Z" fill={N(42, 12)} />
			<path d="M0 800 L400 730 L800 790 L1200 740 L1600 790 V900H0Z" fill={N(50, 5)} />
		{/if}
	{/if}

	<rect width="1600" height="900" fill="url(#vignette-{uid})" />
</svg>

<style>
	.art {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
	}
</style>
