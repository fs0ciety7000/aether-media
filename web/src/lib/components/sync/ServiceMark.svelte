<script lang="ts">
	// Sober service mark: the service's initial on a quiet tile. No brand logo or colour is reproduced.
	interface Props {
		name: string;
		size?: number;
		/** Muted when the service cannot be used yet. */
		muted?: boolean;
	}

	let { name, size = 40, muted = false }: Props = $props();
</script>

<span class="mark" class:muted style:--s="{size}px" aria-hidden="true">{name.charAt(0).toUpperCase()}</span>

<style>
	.mark {
		display: inline-grid;
		place-items: center;
		flex: none;
		width: var(--s);
		height: var(--s);
		border-radius: calc(var(--s) * 0.3);
		background: var(--accent-soft);
		color: var(--accent-ink);
		box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--accent) 22%, transparent);
		font-size: calc(var(--s) * 0.46);
		font-weight: var(--fw-heavy);
		line-height: 1;
		user-select: none;
	}

	.muted {
		background: var(--surface-raised);
		color: var(--text-muted);
		box-shadow: inset 0 0 0 1px var(--line);
	}
</style>
