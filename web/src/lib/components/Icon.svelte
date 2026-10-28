<script lang="ts" module>
	// Thin line icons (24 px grid, 1.5 px stroke, round caps). Drawn for Aether, Lucide-style.
	const ICONS = {
		play: '<path d="M7.5 4.8v14.4a.8.8 0 0 0 1.2.7l11-7.2a.8.8 0 0 0 0-1.4l-11-7.2a.8.8 0 0 0-1.2.7Z"/>',
		pause: '<rect x="6.5" y="5" width="3.5" height="14" rx="1"/><rect x="14" y="5" width="3.5" height="14" rx="1"/>',
		search: '<circle cx="11" cy="11" r="6.5"/><path d="m20 20-4.3-4.3"/>',
		home: '<path d="M4 10.2 12 4l8 6.2V19a1 1 0 0 1-1 1h-4.5v-5.5h-5V20H5a1 1 0 0 1-1-1Z"/>',
		film: '<rect x="3.5" y="4" width="17" height="16" rx="2"/><path d="M7.5 4v16M16.5 4v16M3.5 8h4M3.5 12h4M3.5 16h4M16.5 8h4M16.5 12h4M16.5 16h4"/>',
		tv: '<rect x="3" y="7" width="18" height="12.5" rx="2"/><path d="m8.5 3 3.5 4 3.5-4"/>',
		music: '<path d="M9 18V5.6l11-2.1V16"/><circle cx="6.5" cy="18" r="2.5"/><circle cx="17.5" cy="16" r="2.5"/>',
		image: '<rect x="3.5" y="4" width="17" height="16" rx="2"/><circle cx="9" cy="9.5" r="1.5"/><path d="m20.5 15-4.6-4.6L5.2 20"/>',
		settings:
			'<path d="M18.59 9.96 20.78 10.04 20.78 13.96 18.59 14.04 18.1 15.22 19.6 16.82 16.82 19.6 15.22 18.1 14.04 18.59 13.96 20.78 10.04 20.78 9.96 18.59 8.78 18.1 7.18 19.6 4.4 16.82 5.9 15.22 5.41 14.04 3.22 13.96 3.22 10.04 5.41 9.96 5.9 8.78 4.4 7.18 7.18 4.4 8.78 5.9 9.96 5.41 10.04 3.22 13.96 3.22 14.04 5.41 15.22 5.9 16.82 4.4 19.6 7.18 18.1 8.78Z"/><circle cx="12" cy="12" r="2.75"/>',
		user: '<circle cx="12" cy="8.5" r="3.75"/><path d="M5 20c.9-3.6 3.6-5.6 7-5.6s6.1 2 7 5.6"/>',
		'chevron-left': '<path d="m14.5 6-6 6 6 6"/>',
		'chevron-right': '<path d="m9.5 6 6 6-6 6"/>',
		plus: '<path d="M12 5v14M5 12h14"/>',
		check: '<path d="m5 12.5 4.5 4.5L19 7.5"/>',
		heart: '<path d="M12 19.5S4 15 4 9.4A4.2 4.2 0 0 1 12 7.6a4.2 4.2 0 0 1 8 1.8c0 5.6-8 10.1-8 10.1Z"/>',
		x: '<path d="M6.5 6.5l11 11M17.5 6.5l-11 11"/>',
		menu: '<path d="M4 7h16M4 12h16M4 17h16"/>',
		sliders: '<path d="M4 7h8.5M16.5 7H20M4 17h3.5M11.5 17H20"/><circle cx="14.5" cy="7" r="2"/><circle cx="9.5" cy="17" r="2"/>',
		collection: '<rect x="4" y="9" width="16" height="11" rx="1.5"/><path d="M6 6h12M8.5 3.5h7"/>',
		contrast: '<circle cx="12" cy="12" r="8"/><path d="M12 4a8 8 0 0 1 0 16Z" fill="currentColor" stroke="none"/>',
		panel: '<rect x="3.5" y="4.5" width="17" height="15" rx="2"/><path d="M9.5 4.5v15"/>',
		info: '<circle cx="12" cy="12" r="8"/><path d="M12 11v5"/><circle cx="12" cy="8" r=".6" fill="currentColor"/>',
		compass: '<rect x="3.5" y="3.5" width="17" height="17" rx="4.5"/><path d="m15.2 8.8-1.9 4.5-4.5 1.9 1.9-4.5Z"/>',
		genres: '<path d="M4 6.5h11M4 11.5h7M4 16.5h7"/><path d="M15 12.2v5.6a.5.5 0 0 0 .8.4l4-2.8a.5.5 0 0 0 0-.8l-4-2.8a.5.5 0 0 0-.8.4Z"/>',
		clock: '<path d="M4.6 9.2A8 8 0 1 1 4 12"/><path d="M4 5.5v3.7h3.7"/><path d="M12 8v4.2l2.8 1.8"/>',
		monitor: '<rect x="3" y="4.5" width="18" height="12.5" rx="2.5"/><path d="M8.5 20h7"/><path d="M10.6 8.6v4.8l4-2.4Z"/>',
		layers: '<rect x="8" y="8" width="12" height="12" rx="2.5"/><path d="M16 5.5A2.5 2.5 0 0 0 13.5 3h-8A2.5 2.5 0 0 0 3 5.5v8A2.5 2.5 0 0 0 5.5 16"/>',
		folder: '<path d="M3.5 7.5A2 2 0 0 1 5.5 5.5h3.6l2 2.2h7.4a2 2 0 0 1 2 2V17a2 2 0 0 1-2 2h-13a2 2 0 0 1-2-2Z"/><path d="M3.5 10.5h17"/>',
		bell: '<path d="M6.5 10a5.5 5.5 0 0 1 11 0v3.6l1.6 2.6a.5.5 0 0 1-.4.8H5.3a.5.5 0 0 1-.4-.8l1.6-2.6Z"/><path d="M10 19.5a2.2 2.2 0 0 0 4 0"/>',
		download: '<path d="M12 4v10.5M7.5 10.5 12 15l4.5-4.5"/><path d="M4.5 16v1.5a2 2 0 0 0 2 2h11a2 2 0 0 0 2-2V16"/>',
		trending: '<path d="m3.5 16.5 5.5-5.5 4 4 7.5-7.5"/><path d="M15 7.5h5.5V13"/>',
		'arrow-right': '<path d="M5 12h14M13.5 6.5 19 12l-5.5 5.5"/>',
		'chevron-down': '<path d="m6 9.5 6 6 6-6"/>',
		disc: '<circle cx="12" cy="12" r="8"/><circle cx="12" cy="12" r="2.25"/><path d="M12 6.5a5.5 5.5 0 0 0-5.5 5.5"/>',
		more: '<circle cx="5.5" cy="12" r="1.1" fill="currentColor"/><circle cx="12" cy="12" r="1.1" fill="currentColor"/><circle cx="18.5" cy="12" r="1.1" fill="currentColor"/>',
		lock: '<rect x="5" y="10.5" width="14" height="9.5" rx="2"/><path d="M8.5 10.5V8a3.5 3.5 0 0 1 7 0v2.5"/>',
		unlock: '<rect x="5" y="10.5" width="14" height="9.5" rx="2"/><path d="M8.5 10.5V8a3.5 3.5 0 0 1 6.8-1.2"/>',
		trash: '<path d="M4.5 7h15M9.5 7V5a1 1 0 0 1 1-1h3a1 1 0 0 1 1 1v2"/><path d="M6.5 7l.8 11.2a2 2 0 0 0 2 1.8h5.4a2 2 0 0 0 2-1.8L17.5 7"/>',
		refresh: '<path d="M19.4 14.6A7.5 7.5 0 1 1 18 7.4"/><path d="M19 4v4h-4"/>',
		edit: '<path d="M14.5 5.5 18.5 9.5"/><path d="M5 19l1-4L16 5a1.4 1.4 0 0 1 2 0l1 1a1.4 1.4 0 0 1 0 2L9 18Z"/>',
		alert: '<path d="M12 4.5 20.5 19h-17Z"/><path d="M12 10v4"/><circle cx="12" cy="16.6" r=".6" fill="currentColor"/>',
		'sort-asc': '<path d="M7 18V6M3.5 9.5 7 6l3.5 3.5"/><path d="M13 7h7M13 12h5M13 17h3"/>',
		'sort-desc': '<path d="M7 6v12M3.5 14.5 7 18l3.5-3.5"/><path d="M13 7h3M13 12h5M13 17h7"/>',
		star: '<path d="m12 4.5 2.3 4.7 5.2.8-3.8 3.6.9 5.1L12 16.3l-4.6 2.4.9-5.1-3.8-3.6 5.2-.8Z"/>',
		grid: '<rect x="4" y="4" width="6.5" height="6.5" rx="1.5"/><rect x="13.5" y="4" width="6.5" height="6.5" rx="1.5"/><rect x="4" y="13.5" width="6.5" height="6.5" rx="1.5"/><rect x="13.5" y="13.5" width="6.5" height="6.5" rx="1.5"/>',
		wand: '<path d="m4.5 19.5 10-10"/><path d="m13 8 3 3"/><path d="M17.5 3.5v3M16 5h3M19.5 10.5v2M18.5 11.5h2M9.5 3.5v2M8.5 4.5h2"/>',
		key: '<circle cx="8" cy="15" r="3.5"/><path d="m10.5 12.5 8-8M16 7l2.5 2.5M14 9l1.5 1.5"/>',
		// Player
		volume: '<path d="M4 9.5h3l4.5-4v13l-4.5-4H4a.5.5 0 0 1-.5-.5v-4a.5.5 0 0 1 .5-.5Z"/><path d="M15.5 9a4 4 0 0 1 0 6"/><path d="M18 6.5a7.5 7.5 0 0 1 0 11"/>',
		'volume-low': '<path d="M4 9.5h3l4.5-4v13l-4.5-4H4a.5.5 0 0 1-.5-.5v-4a.5.5 0 0 1 .5-.5Z"/><path d="M15.5 9a4 4 0 0 1 0 6"/>',
		'volume-x': '<path d="M4 9.5h3l4.5-4v13l-4.5-4H4a.5.5 0 0 1-.5-.5v-4a.5.5 0 0 1 .5-.5Z"/><path d="m16 9.5 5 5M21 9.5l-5 5"/>',
		maximize: '<path d="M4 9V5.5A1.5 1.5 0 0 1 5.5 4H9M15 4h3.5A1.5 1.5 0 0 1 20 5.5V9M20 15v3.5a1.5 1.5 0 0 1-1.5 1.5H15M9 20H5.5A1.5 1.5 0 0 1 4 18.5V15"/>',
		minimize: '<path d="M9 4v3.5A1.5 1.5 0 0 1 7.5 9H4M20 9h-3.5A1.5 1.5 0 0 1 15 7.5V4M15 20v-3.5a1.5 1.5 0 0 1 1.5-1.5H20M4 15h3.5A1.5 1.5 0 0 1 9 16.5V20"/>',
		captions: '<rect x="3" y="5" width="18" height="14" rx="2.5"/><path d="M10.5 10.2a2.2 2.2 0 1 0 0 3.6M16.5 10.2a2.2 2.2 0 1 0 0 3.6"/>',
		'skip-next': '<path d="M6 5.5v13a.6.6 0 0 0 .9.5l9.6-6.5a.6.6 0 0 0 0-1L6.9 5a.6.6 0 0 0-.9.5Z"/><path d="M19 5v14"/>',
		'back-10': '<path d="M4.5 12a7.5 7.5 0 1 0 2.2-5.3"/><path d="M4.5 3.5v3.7h3.7"/><path d="M9.6 10v5.5"/><rect x="12" y="10" width="3.4" height="5.5" rx="1.7"/>',
		'forward-10': '<path d="M19.5 12a7.5 7.5 0 1 1-2.2-5.3"/><path d="M19.5 3.5v3.7h-3.7"/><path d="M8.6 10v5.5"/><rect x="11" y="10" width="3.4" height="5.5" rx="1.7"/>',
		'arrow-left': '<path d="M19 12H5M10.5 6.5 5 12l5.5 5.5"/>',
		audio: '<path d="M4 10v4M8 7v10M12 4v16M16 8v8M20 10.5v3"/>',
		gauge: '<path d="M4.5 17a8 8 0 1 1 15 0"/><path d="m12 13 3.5-4"/>'
	} as const;

	export type IconName = keyof typeof ICONS;
	export const ICON_NAMES = Object.keys(ICONS) as IconName[];
</script>

<script lang="ts">
	interface Props {
		name: IconName;
		size?: number;
		/** Filled glyph — reserved for active states (favourite, playing). */
		filled?: boolean;
		/** Accessible label; decorative (aria-hidden) when omitted. */
		label?: string;
		class?: string;
	}

	let { name, size = 20, filled = false, label, class: className }: Props = $props();
</script>

<svg
	class={['icon', className]}
	width={size}
	height={size}
	viewBox="0 0 24 24"
	fill={filled ? 'currentColor' : 'none'}
	stroke="currentColor"
	stroke-width="1.5"
	stroke-linecap="round"
	stroke-linejoin="round"
	role={label ? 'img' : undefined}
	aria-label={label}
	aria-hidden={label ? undefined : 'true'}
	focusable="false"
>
	{@html ICONS[name]}
</svg>

<style>
	.icon {
		flex: none;
	}
</style>
