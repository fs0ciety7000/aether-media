<script lang="ts">
	import AdminOnly from '$lib/components/AdminOnly.svelte';

	let { children } = $props();
</script>

<AdminOnly>{@render children()}</AdminOnly>
