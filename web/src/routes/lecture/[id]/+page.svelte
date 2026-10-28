<script lang="ts">
	// Full-screen player route: /lecture/<itemId>[?t=<seconds>&audio=<index>&sub=<index>].
	// The root layout renders it without the app shell.
	import { afterNavigate, goto } from '$app/navigation';
	import { page } from '$app/state';
	import type { ItemDetail } from '$lib/api/types';
	import Player from '$lib/components/player/Player.svelte';

	const id = $derived(Number(page.params.id));
	const num = (k: string) => {
		const v = page.url.searchParams.get(k);
		return v !== null && v !== '' && Number.isFinite(Number(v)) ? Number(v) : undefined;
	};

	// Back goes to the previous page when it was inside the app, else to the item's page.
	let cameFromApp = false;
	afterNavigate(({ from }) => {
		if (from) cameFromApp = true;
	});

	function back(item: ItemDetail | null) {
		if (cameFromApp && history.length > 1) {
			history.back();
			return;
		}
		const target = item?.kind === 'track' && item.parentId ? item.parentId : id;
		goto(Number.isFinite(target) ? `/item/${target}` : '/', { replaceState: true });
	}
</script>

{#key id}
	<Player itemId={id} startSeconds={num('t')} audioIndex={num('audio')} subtitleIndex={num('sub')} onback={back} />
{/key}
