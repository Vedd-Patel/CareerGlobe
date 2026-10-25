<script lang="ts">
	// S28 Profile: what matching uses (skills, titles, experience, city),
	// the stored resume and the job preferences, all editable here.
	import { api, ApiError, type Schemas } from '$lib/api/client';
	import { account } from '$lib/account.svelte';
	import { session } from '$lib/auth.svelte';
	import { loadPrefs, savePrefs } from '$lib/prefs.svelte';
	import { initials, shortDate } from '$lib/format';
	import Icon from '$lib/components/Icon.svelte';
	import ChipEditor from '$lib/components/ChipEditor.svelte';
	import Banner from '$lib/components/Banner.svelte';
	import SkeletonCards from '$lib/components/SkeletonCards.svelte';

	type Resume = { uploaded?: boolean; original_name?: string | null; size_bytes?: number; uploaded_at?: string; parsed_at?: string | null };

	let ready = $state(false);
	let error = $state('');
	let skills = $state<string[]>([]);
	let titles = $state<string[]>([]);
	let education = $state<string[]>([]);
	let years = $state(0);
	let city = $state('');
	let resume = $state<Resume>({});
	let meta = $state<Schemas['Meta'] | null>(null);
	let categories = $state<string[]>([]);
	let cities = $state<string[]>([]);
	let seniority = $state<string[]>([]);
	let anywhere = $state(false);
	let remoteOk = $state(true);
	let minLpa = $state<number | null>(null);
	let saved = $state('');
	let busy = $state(false);

	$effect(() => {
		Promise.all([
			api.get<Schemas['Profile']>('/v1/profile'),
			api.get<Resume>('/v1/resume'),
			loadPrefs(),
			api.get<Schemas['Meta']>('/v1/meta')
		]).then(
			([p, r, pr, m]) => {
				skills = [...(p.skills ?? [])];
				titles = [...(p.titles ?? [])];
				education = [...(p.education ?? [])];
				years = p.years_exp ?? 0;
				city = p.current_city ?? '';
				resume = r;
				meta = m;
				categories = [...(pr.categories ?? [])];
				cities = [...(pr.cities ?? [])];
				seniority = [...(pr.seniority ?? [])];
				anywhere = pr.anywhere ?? false;
				remoteOk = pr.remote_ok ?? true;
				minLpa = pr.min_salary ? pr.min_salary / 100000 : null;
				ready = true;
			},
			(e) => (error = e instanceof ApiError ? e.message : 'Could not load your profile.')
		);
	});

	const toggle = (list: string[], v: string) => (list.includes(v) ? list.filter((x) => x !== v) : [...list, v]);

	async function save() {
		busy = true;
		error = '';
		saved = '';
		try {
			await api.put('/v1/profile', { skills, titles, education, years_exp: years, current_city: city || null });
			await savePrefs({
				categories: categories as Schemas['Category'][], cities, anywhere, remote_ok: remoteOk,
				seniority: seniority as Schemas['Seniority'][], min_salary: minLpa ? Math.round(minLpa * 100000) : null
			});
			saved = 'Saved. Matches update tonight.';
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Could not save.';
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>Profile · CareerGlobe</title></svelte:head>

<main class="page">
	<header style="display: flex; gap: 14px; align-items: center">
		<span class="logo l1" style="width: 56px; height: 56px; border-radius: 18px; font-size: 20px">{initials(account.me?.name ?? session.user?.name ?? session.user?.email)}</span>
		<div style="flex: 1; min-width: 0">
			<h1 class="t-title">{account.me?.name ?? session.user?.name ?? 'Your profile'}</h1>
			<p class="t-caption t-3">{account.me?.email ?? session.user?.email} · joined {shortDate(account.me?.joined_at)}</p>
		</div>
	</header>
	<nav class="chips">
		<a class="chip" href="/inbox"><Icon name="inbox" size={16} />Inbox{#if account.me?.unread_notifications}<span class="badge-new">{account.me.unread_notifications}</span>{/if}</a>
		<a class="chip" href="/settings"><Icon name="settings" size={16} />Settings</a>
		{#if account.me?.is_admin}<a class="chip" href="/admin"><Icon name="shield" size={16} />Admin</a>{/if}
	</nav>

	{#if error}<Banner kind="err">{error}</Banner>{/if}
	{#if !ready}
		{#if !error}<SkeletonCards count={3} />{/if}
	{:else}
		<section class="card" style="align-items: center">
			<Icon name="file" />
			<div style="flex: 1; min-width: 0">
				{#if resume.uploaded}
					<b>{resume.original_name ?? 'Resume'}</b>
					<p class="t-caption t-3">Uploaded {shortDate(resume.uploaded_at)} · stored encrypted</p>
				{:else}
					<b>No resume stored</b>
				{/if}
			</div>
			<a class="btn btn-secondary" href="/onboarding/resume?from=profile">{resume.uploaded ? 'Replace' : 'Upload'}</a>
		</section>

		<section class="stack">
			<h2 class="t-heading">What we match on</h2>
			<ChipEditor bind:items={skills} label="Skills" placeholder="Add a skill" />
			<ChipEditor bind:items={titles} label="Job titles" placeholder="Add a title" max={10} />
			<ChipEditor bind:items={education} label="Education" placeholder="Add a degree" max={10} />
			<div class="field">
				<span class="t-caption t-2">YEARS OF EXPERIENCE</span>
				<div class="stepper">
					<button type="button" aria-label="Fewer years" onclick={() => (years = Math.max(0, Math.round((years - 1) * 10) / 10))}>−</button>
					<span aria-live="polite">{years} {years === 1 ? 'year' : 'years'}</span>
					<button type="button" aria-label="More years" onclick={() => (years = Math.min(60, Math.round((years + 1) * 10) / 10))}>+</button>
				</div>
			</div>
			<div class="field"><label for="city">Current city</label><input id="city" class="input" bind:value={city} /></div>
		</section>

		<section class="stack">
			<h2 class="t-heading">Jobs you want</h2>
			<span class="t-caption t-2">ROLES</span>
			<div class="chips">
				{#each meta?.categories ?? [] as c (c)}
					<button class="chip {categories.includes(c) ? 'on' : ''}" aria-pressed={categories.includes(c)} onclick={() => (categories = toggle(categories, c))}>{c}</button>
				{/each}
			</div>
			<span class="t-caption t-2">LEVEL</span>
			<div class="chips">
				{#each meta?.seniority ?? [] as s (s)}
					<button class="chip {seniority.includes(s) ? 'on' : ''}" aria-pressed={seniority.includes(s)} onclick={() => (seniority = toggle(seniority, s))}>{s}</button>
				{/each}
			</div>
			<label class="check"><input type="checkbox" bind:checked={anywhere} />Anywhere in India</label>
			{#if !anywhere}<ChipEditor bind:items={cities} label="Cities" placeholder="Add a city" max={20} />{/if}
			<label class="check"><input type="checkbox" bind:checked={remoteOk} />Include remote jobs</label>
			<div class="field">
				<label for="sal">Minimum salary (LPA, only filters jobs that state one)</label>
				<input id="sal" class="input" type="number" min="0" max="500" step="0.5" bind:value={minLpa} />
			</div>
		</section>

		{#if saved}<Banner kind="ok">{saved}</Banner>{/if}
		<button class="btn btn-primary btn-block" disabled={busy} onclick={save}>Save changes</button>
	{/if}
</main>
