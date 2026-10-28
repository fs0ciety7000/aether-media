<script lang="ts">
	import type { Person } from '$lib/api/types';
	import Avatar from '../Avatar.svelte';
	import Row from '../Row.svelte';
	import { hueFrom } from '$lib/media';

	interface Props {
		people: Person[];
		title?: string;
	}

	let { people, title = 'Distribution' }: Props = $props();
</script>

<Row {title} items={people} key={(p, i) => `${p.id}-${i}`} itemWidth="clamp(112px, 9vw, 132px)">
	{#snippet item(p)}
		<a class="person" href="/parcourir?personne={p.id}&nom={encodeURIComponent(p.name)}">
			<Avatar name={p.name} src={p.image} size={96} hue={hueFrom(p.name)} />
			<span class="name">{p.name}</span>
			{#if p.role}<span class="role">{p.role}</span>{/if}
		</a>
	{/snippet}
</Row>

<style>
	.person {
		display: grid;
		justify-items: center;
		gap: 2px;
		padding: var(--sp-2) var(--sp-1);
		border-radius: var(--r-lg);
		text-align: center;
		transition: background-color var(--dur-fast) var(--ease);
	}

	.person:hover {
		background: color-mix(in srgb, var(--text) 5%, transparent);
	}

	.person :global(.avatar) {
		margin-bottom: var(--sp-2);
	}

	.name {
		max-width: 100%;
		font-size: var(--fs-small);
		font-weight: var(--fw-bold);
		color: var(--text);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.role {
		max-width: 100%;
		font-size: var(--fs-micro);
		color: var(--text-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
</style>
