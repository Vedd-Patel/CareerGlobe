<script lang="ts">
	// S05 Log in. No code needed again; a revoked code shows only the notice.
	import { page } from '$app/state';
	import { ApiError } from '$lib/api/client';
	import { signInGoogle, signInApple, signInEmail, signOut, authMessage, authAvailable } from '$lib/auth.svelte';
	import { routeAfterSignIn } from '$lib/flows/after-signin';
	import Banner from '$lib/components/Banner.svelte';

	let email = $state('');
	let password = $state('');
	let busy = $state(false);
	let error = $state('');
	const removed = $derived(page.url.searchParams.get('removed') === '1');

	$effect(() => {
		if (removed) signOut();
	});

	async function run(fn: () => Promise<void>) {
		error = '';
		busy = true;
		try {
			await fn();
			await routeAfterSignIn();
		} catch (err) {
			error = err instanceof ApiError ? err.message : authMessage(err);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>Log in · CareerGlobe</title></svelte:head>

<main class="center">
	{#if removed}
		<div class="card box" style="border-color: var(--rejected)">
			<h1 class="t-title" style="color: var(--rejected)">Your access has been removed</h1>
			<p class="t-body t-2">The invite code for this account was revoked. Contact the person who invited you.</p>
			<a class="btn btn-secondary" href="/">Back to home</a>
		</div>
	{:else}
		<div class="card box">
			<h1 class="t-title">Welcome back</h1>
			{#if !authAvailable()}<Banner kind="warn">Sign-in is not set up on this server yet.</Banner>{/if}
			<button class="btn btn-secondary btn-block" disabled={busy} onclick={() => run(signInGoogle)}><b>G</b> Continue with Google</button>
			<button class="btn btn-secondary btn-block" disabled={busy} onclick={() => run(signInApple)}>Continue with Apple</button>
			<form onsubmit={(e) => { e.preventDefault(); run(() => signInEmail(email, password)); }} style="display: flex; flex-direction: column; gap: 14px">
				<div class="field"><label for="email">Email</label><input id="email" class="input" type="email" bind:value={email} autocomplete="email" required /></div>
				<div class="field"><label for="pw">Password</label><input id="pw" class="input" type="password" bind:value={password} autocomplete="current-password" required /></div>
				<a class="btn btn-text" href="/reset" style="align-self: flex-end">Forgot password?</a>
				{#if error}<Banner kind="err">{error}</Banner>{/if}
				<button class="btn btn-primary btn-block" disabled={busy}>{busy ? 'Logging in…' : 'Log in'}</button>
			</form>
			<p class="t-caption t-3">New here? <a href="/access">Enter an access code</a></p>
		</div>
	{/if}
</main>

<style>
	.center { min-height: 100vh; display: flex; align-items: center; justify-content: center; padding: 24px 16px; background: var(--bg); }
	.box { width: 420px; max-width: 100%; flex-direction: column; gap: 16px; padding: 32px; }
</style>
