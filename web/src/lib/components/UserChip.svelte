<script lang="ts">
	// Signed-in user block for the top bar: avatar + name + small muted detail (email or role).
	import Avatar from './Avatar.svelte';

	interface Props {
		name: string;
		detail?: string;
		src?: string;
		/** Link target; null renders a plain block (inside a menu trigger). */
		href?: string | null;
		/** Avatar only (mobile header). */
		compact?: boolean;
		hue?: number;
	}

	let { name, detail, src, href = '/profil', compact = false, hue }: Props = $props();
</script>

<svelte:element this={href ? 'a' : 'span'} class="user" class:compact href={href ?? undefined} aria-label={href ? `Profil de ${name}` : undefined}>
	<Avatar {name} {src} {hue} size={compact ? 34 : 40} />
	{#if !compact}
		<span class="text">
			<span class="name">{name}</span>
			{#if detail}<span class="detail">{detail}</span>{/if}
		</span>
	{/if}
</svelte:element>

<style>
	.user {
		display: inline-flex;
		align-items: center;
		gap: var(--sp-3);
		min-width: 0;
		padding: 4px 12px 4px 4px;
		border-radius: var(--r-full);
		transition: background-color var(--dur-fast) var(--ease);
	}

	.user.compact {
		padding: 3px;
	}

	.user:hover {
		background: color-mix(in srgb, var(--text) 5%, transparent);
	}

	.text {
		display: grid;
		min-width: 0;
		line-height: 1.3;
	}

	.name {
		font-size: var(--fs-body);
		font-weight: var(--fw-bold);
		color: var(--text);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.detail {
		font-size: var(--fs-micro);
		color: var(--text-muted);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
</style>
