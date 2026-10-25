<script lang="ts">
	import '$lib/styles/app.css';
	import { onMount, type Snippet } from 'svelte';
	import { initAuth } from '$lib/auth.svelte';
	import { applyTheme, savedTheme } from '$lib/theme';

	let { children }: { children: Snippet } = $props();
	let online = $state(true);

	onMount(() => {
		initAuth();
		applyTheme(savedTheme());
		online = navigator.onLine;
		const up = () => (online = true);
		const down = () => (online = false);
		addEventListener('online', up);
		addEventListener('offline', down);
		return () => {
			removeEventListener('online', up);
			removeEventListener('offline', down);
		};
	});
</script>

{#if !online}
	<div class="offline" role="status">You're offline. Showing what was saved.</div>
{/if}
{@render children()}

<style>
	.offline {
		position: fixed; top: 0; left: 0; right: 0; z-index: 50; text-align: center; padding: 8px 16px;
		font: var(--font-caption); background: var(--withdrawn-soft); color: var(--text-2);
	}
</style>
