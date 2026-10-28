<script lang="ts">
	// Profile sections: account (preferences, password, devices, appearance) · synchronisation.
	import SubNav from '$lib/components/shell/SubNav.svelte';
	import { profileNav } from '$lib/nav';

	let { children } = $props();
</script>

<SubNav items={profileNav} label="Profil" exact={['/profil']} />
{@render children()}
