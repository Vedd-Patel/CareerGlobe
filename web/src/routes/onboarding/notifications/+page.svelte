<script lang="ts">
	// S13 Notifications: the 8 AM "new matches" push; Allow / Not now.
	import { goto } from '$app/navigation';
	import { loadMe } from '$lib/account.svelte';
	import { enablePush } from '$lib/push';
	import { savePrefs } from '$lib/prefs.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import Banner from '$lib/components/Banner.svelte';

	let note = $state('');
	async function finish(allow: boolean) {
		await savePrefs({ push_enabled: allow });
		if (allow) note = await enablePush();
		await loadMe();
		if (!note) goto('/globe');
	}
</script>

<div style="display: flex; flex-direction: column; align-items: center; text-align: center; gap: 16px; padding-top: 32px">
	<div style="width: 96px; height: 96px; border-radius: 30px; background: var(--accent-soft); color: var(--accent); display: flex; align-items: center; justify-content: center"><Icon name="bell" size={44} /></div>
	<h1 class="t-title">New matches at 8 AM</h1>
	<p class="t-body t-2">One quiet push each morning, only when there is something new for you. Never more than one a day.</p>
</div>
{#if note}<Banner kind="warn">{note}</Banner><a class="btn btn-secondary btn-block" href="/globe">Continue</a>{/if}
<button class="btn btn-primary btn-block" onclick={() => finish(true)}>Allow notifications</button>
<button class="btn btn-text btn-block" onclick={() => finish(false)}>Not now</button>
