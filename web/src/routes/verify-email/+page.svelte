<script lang="ts">
	// S06 Verify email: resend with a timer; continue once verified.
	import { onMount } from 'svelte';
	import { session, resendVerification, reloadUser, authMessage } from '$lib/auth.svelte';
	import { routeAfterSignIn } from '$lib/flows/after-signin';
	import Banner from '$lib/components/Banner.svelte';
	import Icon from '$lib/components/Icon.svelte';

	let wait = $state(60);
	let error = $state('');
	onMount(() => {
		const t = setInterval(() => (wait = Math.max(0, wait - 1)), 1000);
		return () => clearInterval(t);
	});
	async function resend() {
		try {
			await resendVerification();
			wait = 60;
		} catch (e) {
			error = authMessage(e);
		}
	}
	async function done() {
		await reloadUser();
		if (session.user?.emailVerified) await routeAfterSignIn();
		else error = 'Not verified yet. Open the link in the email first.';
	}
</script>

<main class="center">
	<div class="card box" style="align-items: center; text-align: center">
		<div class="badge"><Icon name="mail" size={40} /></div>
		<h1 class="t-title">Check your inbox</h1>
		<p class="t-body t-2">We sent a link to <b style="color: var(--text)">{session.user?.email ?? 'your email'}</b>. Open it to finish signing up.</p>
		{#if error}<Banner kind="err">{error}</Banner>{/if}
		<button class="btn btn-primary btn-block" onclick={done}>I've verified my email</button>
		<button class="btn btn-secondary btn-block" disabled={wait > 0} onclick={resend}>{wait > 0 ? `Resend in 0:${String(wait).padStart(2, '0')}` : 'Resend email'}</button>
	</div>
</main>

<style>
	.center { min-height: 100vh; display: flex; align-items: center; justify-content: center; padding: 24px 16px; }
	.box { width: 420px; max-width: 100%; flex-direction: column; gap: 16px; padding: 32px; }
	.badge { width: 88px; height: 88px; border-radius: 28px; background: var(--accent-soft); color: var(--accent); display: flex; align-items: center; justify-content: center; }
</style>
