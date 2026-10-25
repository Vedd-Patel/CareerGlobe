<script lang="ts">
	// S29 Settings: theme, daily push, consents, log out, delete account.
	import { goto } from '$app/navigation';
	import { api, ApiError } from '$lib/api/client';
	import { signOut } from '$lib/auth.svelte';
	import { loadPrefs, savePrefs } from '$lib/prefs.svelte';
	import { enablePush } from '$lib/push';
	import { applyTheme, savedTheme, type Theme } from '$lib/theme';
	import { shortDate } from '$lib/format';
	import Icon from '$lib/components/Icon.svelte';
	import Banner from '$lib/components/Banner.svelte';

	type Consents = { consent_text?: string; items?: { policy_version?: string; accepted_at?: string }[] };

	let theme = $state<Theme>(savedTheme());
	let pushOn = $state(false);
	let pushTime = $state('08:00');
	let consents = $state<Consents>({});
	let note = $state('');
	let error = $state('');
	let confirmText = $state('');
	let deleting = $state(false);
	let showDelete = $state(false);

	$effect(() => {
		loadPrefs().then((p) => {
			pushOn = p.push_enabled ?? false;
			pushTime = p.push_time ?? '08:00';
			if (p.theme && p.theme !== theme) {
				theme = p.theme;
				applyTheme(theme);
			}
		}).catch(() => {});
		api.get<Consents>('/v1/consents').then((c) => (consents = c)).catch(() => {});
	});

	async function setTheme(t: Theme) {
		theme = t;
		applyTheme(t);
		await savePrefs({ theme: t }).catch(() => {});
	}

	async function setPush(on: boolean) {
		note = '';
		pushOn = on;
		try {
			await savePrefs({ push_enabled: on, push_time: pushTime });
			if (on) note = await enablePush();
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Could not save.';
		}
	}

	async function savePushTime() {
		await savePrefs({ push_time: pushTime }).catch(() => (error = 'Could not save the time.'));
	}

	async function logOut() {
		await signOut();
		goto('/login');
	}

	async function deleteAccount() {
		if (confirmText !== 'DELETE') return;
		deleting = true;
		error = '';
		try {
			await api.del<{ deleted?: boolean }>('/v1/account', { confirm: 'DELETE' });
			await signOut();
			goto('/');
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Could not delete the account.';
			deleting = false;
		}
	}

	const THEMES: { key: Theme; label: string }[] = [{ key: 'system', label: 'System' }, { key: 'light', label: 'Light' }, { key: 'dark', label: 'Dark' }];
</script>

<svelte:head><title>Settings · CareerGlobe</title></svelte:head>

<main class="page">
	<a class="btn btn-text" style="align-self: flex-start" href="/profile"><Icon name="back" size={18} />Profile</a>
	<h1 class="t-title">Settings</h1>
	{#if error}<Banner kind="err">{error}</Banner>{/if}

	<section class="stack">
		<h2 class="t-heading">Appearance</h2>
		<div class="segs">
			{#each THEMES as t (t.key)}
				<button class="seg {theme === t.key ? 'on' : ''}" aria-pressed={theme === t.key} onclick={() => setTheme(t.key)}>{t.label}</button>
			{/each}
		</div>
	</section>

	<section class="stack">
		<h2 class="t-heading">Notifications</h2>
		<label class="row-toggle">
			<span><b>Daily new matches</b><br /><span class="t-caption t-3">One push a day, only when there's something new.</span></span>
			<input type="checkbox" class="sr-only" checked={pushOn} onchange={(e) => setPush(e.currentTarget.checked)} />
			<span class="toggle {pushOn ? '' : 'off'}" aria-hidden="true"></span>
		</label>
		{#if pushOn}
			<div class="field"><label for="pt">Time (IST)</label><input id="pt" class="input" type="time" bind:value={pushTime} onchange={savePushTime} /></div>
		{/if}
		{#if note}<Banner kind="warn">{note}</Banner>{/if}
	</section>

	<section class="stack">
		<h2 class="t-heading">Privacy</h2>
		{#each consents.items ?? [] as c (c.policy_version)}
			<p class="t-body t-2">Resume processing consent (policy {c.policy_version}) given {shortDate(c.accepted_at)}.</p>
		{/each}
		{#if consents.consent_text}<p class="t-caption t-3">{consents.consent_text}</p>{/if}
		<div class="chips"><a class="chip" href="/privacy">Privacy notice</a><a class="chip" href="/terms">Terms</a></div>
	</section>

	<button class="btn btn-secondary" onclick={logOut}><Icon name="logout" size={18} />Log out</button>

	<section class="stack danger">
		<h2 class="t-heading">Delete account</h2>
		<p class="t-body t-2">This deletes your resume, profile, matches, tracker and notifications right away. It can't be undone.</p>
		{#if showDelete}
			<div class="field"><label for="del">Type DELETE to confirm</label><input id="del" class="input" autocomplete="off" bind:value={confirmText} /></div>
			<button class="btn btn-primary danger-btn" disabled={confirmText !== 'DELETE' || deleting} onclick={deleteAccount}>Delete my account</button>
		{:else}
			<button class="btn btn-text" style="align-self: flex-start; color: var(--rejected)" onclick={() => (showDelete = true)}><Icon name="trash" size={16} />Delete account…</button>
		{/if}
	</section>
</main>

<style>
	.row-toggle { display: flex; align-items: center; gap: 12px; justify-content: space-between; cursor: pointer; }
	.row-toggle:focus-within .toggle { outline: 2px solid var(--accent); outline-offset: 2px; }
	.danger { border-top: 1px solid var(--line); padding-top: 16px; }
	.danger-btn { background: var(--rejected); border-color: var(--rejected); color: #fff; }
</style>
