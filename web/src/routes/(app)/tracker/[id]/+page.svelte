<script lang="ts">
	// S24 Application detail: status, next step, notes, interview rounds, timeline.
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api, ApiError, type Schemas } from '$lib/api/client';
	import { shortDate, timeOfDay } from '$lib/format';
	import Icon from '$lib/components/Icon.svelte';
	import Logo from '$lib/components/Logo.svelte';
	import StatusPill from '$lib/components/StatusPill.svelte';
	import Banner from '$lib/components/Banner.svelte';
	import SkeletonCards from '$lib/components/SkeletonCards.svelte';

	type Status = Schemas['ApplicationStatus'];
	type Detail = { application: Schemas['ApplicationCard']; events: Schemas['ApplicationEvent'][]; rounds: Schemas['InterviewRound'][] };
	const STATUSES: Status[] = ['saved', 'applied', 'interview', 'offer', 'rejected', 'withdrawn'];

	let d = $state<Detail | null>(null);
	let error = $state('');
	let notes = $state('');
	let notesSaved = $state(true);
	let nextStep = $state('');
	let nextAt = $state('');
	let roundKind = $state('Technical');
	let roundAt = $state('');
	let roundNotes = $state('');
	let addingRound = $state(false);

	const id = $derived(Number(page.params.id));

	// datetime-local <-> ISO
	const toLocal = (iso: string | null | undefined) => {
		if (!iso) return '';
		const t = new Date(iso);
		return new Date(t.getTime() - t.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
	};
	const fromLocal = (v: string) => new Date(v).toISOString();

	async function load() {
		try {
			const r = await api.get<Detail>(`/v1/applications/${id}`);
			r.events ??= [];
			r.rounds ??= [];
			d = r;
			notes = r.application.notes ?? '';
			nextStep = r.application.next_step ?? '';
			nextAt = toLocal(r.application.next_step_at);
		} catch (e) {
			error = e instanceof ApiError && e.status === 404 ? 'This application is not in your tracker.' : 'Could not load it.';
		}
	}
	$effect(() => {
		void id;
		load();
	});

	async function setStatus(s: Status) {
		if (!d || d.application.status === s) return;
		await api.patch(`/v1/applications/${id}`, { status: s });
		await load();
	}

	let notesTimer: ReturnType<typeof setTimeout> | undefined;
	function onNotes() {
		notesSaved = false;
		clearTimeout(notesTimer);
		notesTimer = setTimeout(async () => {
			await api.patch(`/v1/applications/${id}`, { notes });
			notesSaved = true;
		}, 800);
	}

	async function saveNext() {
		if (!nextStep.trim()) {
			await api.patch(`/v1/applications/${id}`, { clear_next_step: true });
		} else {
			await api.patch(`/v1/applications/${id}`, { next_step: nextStep.trim(), ...(nextAt ? { next_step_at: fromLocal(nextAt) } : {}) });
		}
		await load();
	}

	async function addRound() {
		if (!roundAt || !roundKind.trim()) return;
		await api.post(`/v1/applications/${id}/rounds`, { at: fromLocal(roundAt), kind: roundKind.trim(), notes: roundNotes });
		addingRound = false;
		roundNotes = '';
		roundAt = '';
		await load();
	}

	async function deleteRound(rid: number) {
		await api.del(`/v1/applications/${id}/rounds/${rid}`);
		await load();
	}

	async function remove() {
		if (!confirm('Remove this job from your tracker? Notes and rounds are deleted too.')) return;
		await api.del(`/v1/applications/${id}`);
		goto('/tracker');
	}
</script>

<svelte:head><title>{d?.application.title ?? 'Application'} · CareerGlobe</title></svelte:head>

<main class="page">
	<a class="btn btn-text" style="align-self: flex-start" href="/tracker"><Icon name="back" size={18} />Tracker</a>
	{#if error}
		<Banner kind="err">{error}</Banner>
	{:else if !d}
		<SkeletonCards count={3} />
	{:else}
		{@const a = d.application}
		<header style="display: flex; gap: 14px; align-items: center">
			<Logo name={a.company_name ?? ''} url={a.logo_url} size="lg" />
			<div style="flex: 1; min-width: 0">
				<h1 class="t-title">{a.title}</h1>
				<p class="t-body t-2">{a.company_name}{a.cities?.length ? ` · ${a.cities.join(', ')}` : ''}</p>
			</div>
			<StatusPill status={a.status ?? 'saved'} />
		</header>
		{#if a.job_status === 'closed'}<Banner kind="warn">The company closed this job on {shortDate(a.job_closed_at)}.</Banner>{/if}
		<div style="display: flex; gap: 10px; flex-wrap: wrap">
			<a class="btn btn-secondary" href="/jobs/{a.job_id}">View job</a>
			{#if a.apply_url}<a class="btn btn-secondary" href={a.apply_url} target="_blank" rel="noopener noreferrer">Company page<Icon name="external" size={16} /></a>{/if}
		</div>

		<section class="stack">
			<h2 class="t-heading">Status</h2>
			<div class="chips">
				{#each STATUSES as s (s)}
					<button class="chip {a.status === s ? 'on' : ''}" aria-pressed={a.status === s} onclick={() => setStatus(s)}>{s.charAt(0).toUpperCase() + s.slice(1)}</button>
				{/each}
			</div>
		</section>

		<section class="stack">
			<h2 class="t-heading">Next step</h2>
			<div class="field"><label for="ns">What's next</label><input id="ns" class="input" placeholder="e.g. Send portfolio" bind:value={nextStep} /></div>
			<div class="field"><label for="na">When</label><input id="na" class="input" type="datetime-local" bind:value={nextAt} /></div>
			<button class="btn btn-secondary" style="align-self: flex-start" onclick={saveNext}>Save next step</button>
			<p class="t-caption t-3">You'll get a reminder on the morning of the day.</p>
		</section>

		<section class="stack">
			<div style="display: flex; justify-content: space-between; align-items: center">
				<h2 class="t-heading">Interview rounds</h2>
				<button class="btn btn-text" onclick={() => (addingRound = !addingRound)}><Icon name="plus" size={16} />Add round</button>
			</div>
			{#if addingRound}
				<div class="card" style="flex-direction: column; align-items: stretch; gap: 10px; box-shadow: none">
					<div class="field"><label for="rk">Kind</label><input id="rk" class="input" maxlength="100" bind:value={roundKind} /></div>
					<div class="field"><label for="ra">Date and time</label><input id="ra" class="input" type="datetime-local" bind:value={roundAt} /></div>
					<div class="field"><label for="rn">Notes</label><textarea id="rn" class="input" rows="2" maxlength="5000" bind:value={roundNotes}></textarea></div>
					<button class="btn btn-primary" disabled={!roundAt || !roundKind.trim()} onclick={addRound}>Add</button>
				</div>
			{/if}
			{#each d.rounds as r (r.id)}
				<div class="list-row">
					<Icon name="calendar" />
					<div style="flex: 1">
						<b>{r.kind}</b>
						<p class="t-caption t-3">{shortDate(r.at)} · {timeOfDay(r.at)}</p>
						{#if r.notes}<p class="t-caption t-2">{r.notes}</p>{/if}
					</div>
					<button class="icon-btn" aria-label="Delete round" onclick={() => deleteRound(r.id ?? 0)}><Icon name="trash" size={18} /></button>
				</div>
			{:else}
				<p class="t-body t-3">No rounds yet.</p>
			{/each}
		</section>

		<section class="stack">
			<div style="display: flex; justify-content: space-between; align-items: center">
				<h2 class="t-heading">Notes</h2><span class="t-caption t-3">{notesSaved ? 'Saved' : 'Saving…'}</span>
			</div>
			<textarea class="input" rows="6" maxlength="20000" placeholder="Recruiter name, salary discussed, questions asked…" bind:value={notes} oninput={onNotes}></textarea>
		</section>

		<section class="stack">
			<h2 class="t-heading">Timeline</h2>
			{#each d.events as ev (ev.id)}
				<div class="list-row">
					<StatusPill status={ev.to_status ?? ''} />
					<span class="t-caption t-2" style="flex: 1">{ev.from_status ? `from ${ev.from_status}` : 'added'}{ev.note ? ` · ${ev.note}` : ''}</span>
					<span class="t-caption t-3">{shortDate(ev.at)}</span>
				</div>
			{/each}
		</section>

		<button class="btn btn-text" style="align-self: flex-start; color: var(--rejected)" onclick={remove}><Icon name="trash" size={16} />Remove from tracker</button>
	{/if}
</main>
