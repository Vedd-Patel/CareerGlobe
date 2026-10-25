<script lang="ts">
	// S20 Job detail with the match breakdown, S21 "Did you apply?" on return,
	// S22 closed job with similar open jobs.
	import { page } from '$app/state';
	import { api, ApiError, type Schemas } from '$lib/api/client';
	import { ago, salary, shortDate, workMode } from '$lib/format';
	import Icon from '$lib/components/Icon.svelte';
	import Logo from '$lib/components/Logo.svelte';
	import ScoreRing from '$lib/components/ScoreRing.svelte';
	import StatusPill from '$lib/components/StatusPill.svelte';
	import JobCard from '$lib/components/JobCard.svelte';
	import Banner from '$lib/components/Banner.svelte';
	import SkeletonCards from '$lib/components/SkeletonCards.svelte';

	type Job = Schemas['JobDetail'];
	const PARTS: [string, string, number][] = [
		['skills', 'Skills', 45], ['role', 'Role', 20], ['seniority', 'Seniority', 15], ['location', 'Location', 15], ['freshness', 'Freshness', 5]
	];

	let job = $state<Job | null>(null);
	let error = $state('');
	let askApplied = $state(false);
	let leftAt = 0;
	let reporting = $state(false);
	let report = $state('');
	let reported = $state(false);

	const id = $derived(Number(page.params.id));

	async function load() {
		error = '';
		try {
			job = await api.get<Job>(`/v1/jobs/${id}`);
			if (job.match) api.post(`/v1/matches/${id}/seen`).catch(() => {});
		} catch (e) {
			error = e instanceof ApiError && e.status === 404 ? 'This job is not in CareerGlobe.' : 'Could not load the job.';
		}
	}

	$effect(() => {
		void id;
		job = null;
		load();
	});

	function apply() {
		leftAt = Date.now();
	}

	// Coming back to the tab after opening the company's page asks once (S21).
	$effect(() => {
		const onVisible = () => {
			if (document.visibilityState !== 'visible' || !leftAt || !job) return;
			const st = job.application?.status;
			if (Date.now() - leftAt > 3000 && !job.application?.dont_ask && st !== 'applied' && st !== 'interview' && st !== 'offer') {
				askApplied = true;
			}
			leftAt = 0;
		};
		document.addEventListener('visibilitychange', onVisible);
		return () => document.removeEventListener('visibilitychange', onVisible);
	});

	async function setStatus(status: Schemas['ApplicationStatus']) {
		if (!job) return;
		const a = await api.post<Schemas['Application']>('/v1/applications', { job_id: job.id, status });
		job.application = { id: a.id, status: a.status, dont_ask: a.dont_ask };
	}

	async function answer(applied: boolean, dontAsk = false) {
		askApplied = false;
		if (applied) await setStatus('applied');
		else if (dontAsk) {
			if (!job?.application) await setStatus('saved');
			if (job?.application?.id) {
				await api.patch(`/v1/applications/${job.application.id}`, { dont_ask: true });
				job.application.dont_ask = true;
			}
		}
	}

	async function sendReport() {
		if (!job || !report.trim()) return;
		await api.post('/v1/reports', { job_id: job.id, company_id: job.company?.id, message: report.trim() });
		reported = true;
		reporting = false;
	}
</script>

<svelte:head><title>{job?.title ?? 'Job'} · CareerGlobe</title></svelte:head>

<main class="page">
	<button class="btn btn-text" style="align-self: flex-start" onclick={() => history.back()}><Icon name="back" size={18} />Back</button>
	{#if error}
		<Banner kind="err">{error}</Banner>
	{:else if !job}
		<SkeletonCards count={3} />
	{:else}
		{#if job.status === 'closed'}
			<Banner kind="warn">This job closed on {shortDate(job.closed_at)}. It no longer appears on {job.company?.name}'s careers page.</Banner>
		{/if}
		<header class="stack">
			<a class="plain-link" href="/companies/{job.company?.id}" style="display: flex; gap: 12px; align-items: center">
				<Logo name={job.company?.name ?? ''} url={job.company?.logo_url} />
				<span class="t-heading">{job.company?.name}</span>
			</a>
			<h1 class="t-title">{job.title}</h1>
			<p class="t-body t-2">
				{[(job.cities ?? []).join(', '), workMode(job.work_mode), job.seniority, salary(job.salary_min, job.salary_max)].filter(Boolean).join(' · ')}
			</p>
			<p class="t-caption t-3">Posted {ago(job.posted_at ?? job.first_seen)} · Last checked {ago(job.last_verified)}</p>
			{#if job.application}<StatusPill status={job.application.status ?? 'saved'} />{/if}
		</header>

		{#if job.status === 'open'}
			<div style="display: flex; gap: 10px">
				<a class="btn btn-primary" style="flex: 2" href={job.apply_url} target="_blank" rel="noopener noreferrer" onclick={apply}>
					Apply on company site<Icon name="external" size={18} />
				</a>
				<button class="btn btn-secondary" style="flex: 1" disabled={!!job.application} onclick={() => setStatus('saved')}>
					<Icon name="bookmark" size={18} fill={!!job.application} />{job.application ? 'In tracker' : 'Save'}
				</button>
			</div>
		{/if}

		{#if job.match}
			<section class="card" style="flex-direction: column; align-items: stretch; gap: 14px">
				<div style="display: flex; gap: 14px; align-items: center">
					<ScoreRing score={job.match.score} size="lg" />
					<div>
						<h2 class="t-heading">Why it matches</h2>
						<p class="t-body t-2">{job.match.reasons?.summary}</p>
					</div>
				</div>
				{#each PARTS as [key, label, max] (key)}
					{@const got = job.match.reasons?.parts?.[key] ?? 0}
					<div>
						<div style="display: flex; justify-content: space-between" class="t-caption"><span>{label}</span><span class="t-3">{Math.round(got)} / {max}</span></div>
						<div class="progress"><i style="width: {(got / max) * 100}%"></i></div>
					</div>
				{/each}
				{#if job.match.reasons?.skills_have?.length}
					<div class="chips">{#each job.match.reasons.skills_have as s (s)}<span class="skill have">{s}</span>{/each}</div>
				{/if}
				{#if job.match.reasons?.skills_missing?.length}
					<div class="chips">{#each job.match.reasons.skills_missing as s (s)}<span class="skill miss">{s}</span>{/each}</div>
				{/if}
				{#if job.match.reasons?.seniority?.info}<p class="t-caption t-2">{job.match.reasons.seniority.info}</p>{/if}
				{#if job.match.reasons?.location}<p class="t-caption t-2">{job.match.reasons.location}</p>{/if}
				{#if job.match.reasons?.few_skills}<p class="t-caption t-3">This listing names few skills, so the score leans on role and level.</p>{/if}
			</section>
		{/if}

		{#if job.description_excerpt}
			<section class="stack">
				<h2 class="t-heading">About the role</h2>
				<p class="t-body t-2" style="white-space: pre-line">{job.description_excerpt}</p>
				<a class="btn btn-text" style="align-self: flex-start" href={job.apply_url} target="_blank" rel="noopener noreferrer">Read the full listing<Icon name="external" size={16} /></a>
			</section>
		{/if}

		{#if job.similar?.length}
			<section class="stack">
				<h2 class="t-heading">{job.status === 'closed' ? 'Similar open jobs' : 'Similar jobs'}</h2>
				{#each job.similar as s (s.id)}
					<JobCard job={{ ...s, job_id: s.id ?? 0, title: s.title ?? '' }} />
				{/each}
			</section>
		{/if}

		{#if reported}
			<Banner kind="ok">Thanks. We'll check it.</Banner>
		{:else if reporting}
			<section class="stack">
				<textarea class="input" rows="3" maxlength="2000" placeholder="What's wrong with this listing?" bind:value={report}></textarea>
				<div style="display: flex; gap: 10px">
					<button class="btn btn-primary" disabled={!report.trim()} onclick={sendReport}>Send</button>
					<button class="btn btn-text" onclick={() => (reporting = false)}>Cancel</button>
				</div>
			</section>
		{:else}
			<button class="btn btn-text" style="align-self: flex-start" onclick={() => (reporting = true)}><Icon name="alert" size={16} />Report wrong info</button>
		{/if}
	{/if}
</main>

{#if askApplied && job}
	<div class="scrim" role="presentation" onclick={() => (askApplied = false)}></div>
	<div class="sheet ask" role="dialog" aria-modal="true" aria-label="Did you apply?">
		<span class="grabber"></span>
		<h2 class="t-title">Did you apply?</h2>
		<p class="t-body t-2">{job.title} at {job.company?.name}. We'll add it to your tracker.</p>
		<button class="btn btn-primary btn-block" onclick={() => answer(true)}>Yes, I applied</button>
		<button class="btn btn-secondary btn-block" onclick={() => answer(false)}>Not yet</button>
		<button class="btn btn-text" onclick={() => answer(false, true)}>Don't ask for this job</button>
	</div>
{/if}

<style>
	.progress { height: 6px; }
	.scrim { position: fixed; inset: 0; background: rgba(5, 8, 19, 0.5); z-index: 50; }
	.ask { position: fixed; left: 0; right: 0; bottom: 0; z-index: 51; max-width: 520px; margin: 0 auto; padding: 12px 20px 28px; display: flex; flex-direction: column; gap: 12px; }
</style>
