<script lang="ts">
	// S04 Access code + Sign up: code first; nothing else works without it.
	import { goto } from '$app/navigation';
	import { request, ApiError } from '$lib/api/client';
	import { session, signInGoogle, signInApple, signUpEmail, authMessage, authAvailable } from '$lib/auth.svelte';
	import { routeAfterSignIn, saveTicket } from '$lib/flows/after-signin';
	import Banner from '$lib/components/Banner.svelte';
	import { deviceId } from '$lib/format';

	let step = $state<1 | 2>(1);
	let code = $state('');
	let email = $state('');
	let password = $state('');
	let agree = $state(false);
	let busy = $state(false);
	let error = $state('');

	async function verify(e: Event) {
		e.preventDefault();
		error = '';
		busy = true;
		try {
			const r = await request<{ ticket: string }>('POST', '/v1/access/verify', {
				body: { code, device_id: deviceId() }, auth: false
			});
			saveTicket(r.ticket);
			if (session.user) await routeAfterSignIn(); // already signed in: link this account now
			else step = 2;
		} catch (err) {
			error = err instanceof ApiError ? err.message : 'Something went wrong.';
		} finally {
			busy = false;
		}
	}

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

	function emailSignup(e: Event) {
		e.preventDefault();
		if (!agree) return (error = 'Agree to the Terms and Privacy Policy to continue.');
		run(() => signUpEmail(email, password));
	}
</script>

<svelte:head><title>Access code · CareerGlobe</title></svelte:head>

<main class="wrap">
	<div class="space side" aria-hidden="true">
		<div class="globe" style="width: 520px; height: 520px; left: -60px; top: 80px"></div>
		<div class="landmass" style="width: 150px; height: 190px; left: 200px; top: 260px; transform: rotate(18deg)"></div>
		<div class="side-copy"><p class="t-title">Private, invite-only</p><p class="t-body" style="color: #a6b0c9">Your code is checked again on every request, so access stays private.</p></div>
	</div>
	<div class="panel">
		{#if step === 1}
			<form class="card form" onsubmit={verify}>
				<p class="t-caption t-accent">STEP 1 OF 2</p>
				<h1 class="t-title">Enter your access code</h1>
				<p class="t-body t-2">CareerGlobe is invite-only. Use the code you were given, like CG-7K4P-2QXM.</p>
				<div class="field">
					<label for="code">Access code</label>
					<input id="code" class="input code-input" bind:value={code} autocomplete="one-time-code" autocapitalize="characters" spellcheck="false" required maxlength="16" />
				</div>
				{#if error}<Banner kind="err">{error}</Banner>{/if}
				<button class="btn btn-primary btn-block" disabled={busy || code.trim().length < 8}>{busy ? 'Checking…' : 'Continue'}</button>
				<p class="t-caption t-3">Already have an account? <a href="/login">Log in</a> — returning members never re-enter a code.</p>
			</form>
		{:else}
			<div class="card form">
				<p class="t-caption t-accent">STEP 2 OF 2 · CODE ACCEPTED</p>
				<h1 class="t-title">Create your account</h1>
				{#if !authAvailable()}
					<Banner kind="warn">Sign-in is not set up on this server yet.</Banner>
				{/if}
				<button class="btn btn-secondary btn-block" disabled={busy} onclick={() => run(signInGoogle)}><b>G</b> Continue with Google</button>
				<button class="btn btn-secondary btn-block" disabled={busy} onclick={() => run(signInApple)}>Continue with Apple</button>
				<div class="or t-caption t-3"><span></span>or with email<span></span></div>
				<form onsubmit={emailSignup} style="display: flex; flex-direction: column; gap: 14px">
					<div class="field"><label for="email">Email</label><input id="email" class="input" type="email" bind:value={email} autocomplete="email" required /></div>
					<div class="field"><label for="pw">Password</label><input id="pw" class="input" type="password" bind:value={password} autocomplete="new-password" minlength="8" required /></div>
					<label class="check"><input type="checkbox" bind:checked={agree} />I agree to the <a href="/terms">Terms</a> and <a href="/privacy">Privacy Policy</a>.</label>
					{#if error}<Banner kind="err">{error}</Banner>{/if}
					<button class="btn btn-primary btn-block" disabled={busy}>{busy ? 'Creating…' : 'Create account'}</button>
				</form>
				<button class="btn btn-text" onclick={() => goto('/login')}>Already have an account? Log in</button>
			</div>
		{/if}
	</div>
</main>

<style>
	.wrap { min-height: 100vh; display: flex; flex-wrap: wrap; background: var(--bg); }
	.side { flex: 1 1 480px; min-height: 360px; display: flex; flex-direction: column; justify-content: flex-end; padding: 48px; color: #e9edf7; }
	.side-copy { position: relative; display: flex; flex-direction: column; gap: 8px; max-width: 460px; }
	.panel { flex: 1 1 420px; display: flex; align-items: center; justify-content: center; padding: 40px 16px; }
	.form { width: 440px; max-width: 100%; flex-direction: column; gap: 16px; padding: 32px; }
	.or { display: flex; align-items: center; gap: 10px; }
	.or span { flex: 1; height: 1px; background: var(--line); }
	@media (max-width: 700px) { .side { display: none; } }
</style>
