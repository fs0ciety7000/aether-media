<script lang="ts">
	// "Hors ligne" badge over artwork / in lists: the item's files are on an offline source.
	import Tag from './Tag.svelte';

	interface Props {
		/** image = over artwork (default) · surface = in a list or on a page. */
		variant?: 'image' | 'surface';
	}

	let { variant = 'image' }: Props = $props();
</script>

<Tag {variant} icon="cloud-off">Hors ligne</Tag>
