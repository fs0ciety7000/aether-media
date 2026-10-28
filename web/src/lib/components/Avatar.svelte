<script lang="ts">
	// Round avatar: image when available, otherwise initials on a soft generated gradient.
	interface Props {
		name: string;
		src?: string;
		size?: number;
		/** Hue of the generated ground (0–360). */
		hue?: number;
	}

	let { name, src, size = 40, hue = 190 }: Props = $props();

	// Server images (/api/v1/…) are resized on request: ask for about twice the
	// displayed size (high-density screens), snapped to the server's widths.
	const WIDTHS = [92, 154, 185, 342];
	const sized = $derived.by(() => {
		if (!src || !src.startsWith('/api/')) return src;
		const want = size * 2;
		const w = WIDTHS.find((x) => x >= want) ?? WIDTHS[WIDTHS.length - 1];
		return src + (src.includes('?') ? '&' : '?') + 'w=' + w;
	});
	let failed = $state(false);
	$effect(() => {
		void sized;
		failed = false;
	});

	const initials = $derived(
		name
			.split(/\s+/)
			.filter(Boolean)
			.slice(0, 2)
			.map((p) => p[0]!.toUpperCase())
			.join('')
	);
</script>

<span
	class="avatar"
	style:--size="{size}px"
	style:--h={hue}
	aria-hidden="true"
>
	{#if sized && !failed}
		<img
			src={sized}
			alt=""
			width={size}
			height={size}
			loading="lazy"
			decoding="async"
			onerror={() => (failed = true)}
		/>
	{:else}
		<span class="initials">{initials}</span>
	{/if}
</span>

<style>
	.avatar {
		position: relative;
		display: inline-grid;
		place-items: center;
		flex: none;
		width: var(--size);
		height: var(--size);
		border-radius: 50%;
		overflow: hidden;
		background:
			radial-gradient(circle at 30% 25%, hsl(calc(var(--h) + 20) 32% 52%), transparent 60%),
			linear-gradient(150deg, hsl(var(--h) 30% 34%), hsl(calc(var(--h) + 30) 32% 18%));
		box-shadow: 0 0 0 2px var(--bg), 0 0 0 3px var(--line-strong);
		color: #f6ffff;
	}

	img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}

	.initials {
		font-size: calc(var(--size) * 0.38);
		font-weight: var(--fw-heavy);
		letter-spacing: 0.02em;
		text-shadow: 0 1px 2px rgba(0, 0, 0, 0.3);
	}
</style>
