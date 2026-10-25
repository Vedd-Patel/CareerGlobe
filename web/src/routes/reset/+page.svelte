<script lang="ts">
	// S07 Forgot / reset password. The answer never reveals whether an account exists.
	import { resetPassword } from '$lib/auth.svelte';
	import Banner from '$lib/components/Banner.svelte';
	let email = $state('');
	let sent = $state(false);
	async function submit(e: Event) {
		e.preventDefault();
		try {
			await resetPassword(email);
		} catch {
			/* same message either way */
		}
		sent = true;
	}
</script>

<main class="center">
	<form class="card box" onsubmit={submit}>
		<a class="btn btn-text" href="/login" style="align-self: flex-start; padding: 0">← Log in</a>
		<h1 class="t-title">Reset your password</h1>
		<p class="t-body t-2">Enter your account email and we'll send a reset link.</p>
		<div class="field"><label for="email">Email</label><input id="email" class="input" type="email" bind:value={email} autocomplete="email" required /></div>
		<button class="btn btn-primary btn-block">Send reset link</button>
		{#if sent}<Banner>If an account exists for this email, a link is on its way.</Banner>{/if}
	</form>
</main>

<style>
	.center { min-height: 100vh; display: flex; align-items: center; justify-content: center; padding: 24px 16px; }
	.box { width: 420px; max-width: 100%; flex-direction: column; gap: 16px; padding: 32px; }
</style>
