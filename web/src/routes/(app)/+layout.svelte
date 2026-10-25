<script lang="ts">
	import type { Snippet } from 'svelte';
	import { session } from '$lib/auth.svelte';
	import { account, loadMe } from '$lib/account.svelte';
	import { requireMember } from '$lib/flows/guard';
	import AppNav from '$lib/components/AppNav.svelte';
	import Banner from '$lib/components/Banner.svelte';

	let { children }: { children: Snippet } = $props();
	let ok = $state(false);

	$effect(() => {
		if (!session.ready) return;
		void session.user;
		requireMember().then((m) => (ok = m));
	});
</script>

{#if ok}
	<AppNav badges={{ '/matches': account.me?.new_matches ?? 0, '/profile': account.me?.unread_notifications ?? 0 }} />
	<div class="app-main">{@render children()}</div>
{:else if account.state === 'error'}
	<main style="padding: 48px 16px; max-width: 480px; margin: 0 auto; display: flex; flex-direction: column; gap: 12px">
		<Banner kind="err">{account.error}</Banner>
		<button class="btn btn-secondary" onclick={() => loadMe()}>Retry</button>
	</main>
{:else}
	<div class="boot" aria-busy="true" aria-label="Loading"><span class="globe-dot"></span></div>
{/if}

<style>
	.app-main { min-height: 100vh; padding-bottom: 84px; }
	@media (min-width: 900px) { .app-main { padding-bottom: 0; padding-left: 88px; } }
	.boot { min-height: 100vh; display: flex; align-items: center; justify-content: center; }
	.globe-dot { width: 48px; height: 48px; border-radius: 50%; background: radial-gradient(circle at 35% 35%, #3dd6c6, #0e2150); animation: pulse 1.2s ease-in-out infinite; }
	@keyframes pulse { 50% { transform: scale(0.85); opacity: 0.6; } }
</style>
