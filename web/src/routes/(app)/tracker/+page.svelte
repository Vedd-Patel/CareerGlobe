<script lang="ts">
	// S23 Tracker board (drag a card or use its menu to move it) and S25 stats.
	import { api, ApiError, type Schemas } from '$lib/api/client';
	import { ago } from '$lib/format';
	import Logo from '$lib/components/Logo.svelte';
	import Banner from '$lib/components/Banner.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import SkeletonCards from '$lib/components/SkeletonCards.svelte';

	type Card = Schemas['ApplicationCard'];
	type Status = Schemas['ApplicationStatus'];
	type Stats = { saved?: number; applied?: number; interview?: number; offer?: number; rejected?: number; withdrawn?: number; response_rate?: number | null };

	const COLUMNS: { key: Status; label: string }[] = [
		{ key: 'saved', label: 'Saved' }, { key: 'applied', label: 'Applied' }, { key: 'interview', label: 'Interview' },
		{ key: 'offer', label: 'Offer' }, { key: 'rejected', label: 'Rejected' }, { key: 'withdrawn', label: 'Withdrawn' }
	];

	let items = $state<Card[] | null>(null);
	let stats = $state<Stats>({});
	let error = $state('');
	let dragging = $state<number | null>(null);
	let over = $state<Status | null>(null);
	let tab = $state<Status>('saved');

	async function load() {
		try {
			const [a, s] = await Promise.all([
				api.get<{ items?: Card[] }>('/v1/applications'),
				api.get<Stats>('/v1/applications/stats')
			]);
			items = a.items ?? [];
			stats = s;
		} catch (e) {
			error = e instanceof ApiError ? e.message : 'Could not load your tracker.';
		}
	}
	$effect(() => {
		load();
	});

	const byStatus = (s: Status) => (items ?? []).filter((c) => c.status === s);

	async function move(id: number, to: Status) {
		const card = items?.find((c) => c.id === id);
		if (!card || card.status === to) return;
		const from = card.status;
		card.status = to;
		try {
			await api.patch(`/v1/applications/${id}`, { status: to });
			stats = await api.get<Stats>('/v1/applications/stats');
		} catch {
			card.status = from;
			error = 'Could not move the card. Try again.';
		}
	}

	function drop(to: Status) {
		if (dragging != null) move(dragging, to);
		dragging = null;
		over = null;
	}
</script>

<svelte:head><title>Tracker · CareerGlobe</title></svelte:head>

<main class="page wide">
	<div class="page-head"><h1 class="t-title">Tracker</h1></div>

	{#if error}<Banner kind="err">{error}</Banner>{/if}

	{#if items === null}
		{#if !error}<SkeletonCards count={4} />{/if}
	{:else if !items.length}
		<EmptyState icon="kanban" title="Nothing tracked yet" tone="applied">
			<p class="t-body t-2">Save a job or tell us you applied, and it shows up here.</p>
			{#snippet actions()}<a class="btn btn-primary" href="/matches">See your matches</a>{/snippet}
		</EmptyState>
	{:else}
		<section class="stats" aria-label="Stats">
			{#each COLUMNS.slice(0, 4) as c (c.key)}
				<div class="card stat"><span class="num">{stats[c.key] ?? 0}</span><span class="t-caption t-3">{c.label}</span></div>
			{/each}
			<div class="card stat">
				<span class="num t-accent">{stats.response_rate == null ? '–' : `${Math.round(stats.response_rate * 100)}%`}</span>
				<span class="t-caption t-3">Response rate</span>
			</div>
		</section>

		<div class="segs tabs" role="tablist">
			{#each COLUMNS as c (c.key)}
				<button class="seg {tab === c.key ? 'on' : ''}" role="tab" aria-selected={tab === c.key} onclick={() => (tab = c.key)}>
					{c.label} {byStatus(c.key).length}
				</button>
			{/each}
		</div>

		<div class="cols">
			{#each COLUMNS as col (col.key)}
				<section class="col {tab === col.key ? 'current' : ''} {over === col.key ? 'over' : ''}" aria-label={col.label}
					ondragover={(e) => { e.preventDefault(); over = col.key; }}
					ondragleave={() => over === col.key && (over = null)}
					ondrop={(e) => { e.preventDefault(); drop(col.key); }}>
					<h2 class="t-caption t-2 col-head">{col.label.toUpperCase()} · {byStatus(col.key).length}</h2>
					{#each byStatus(col.key) as card (card.id)}
						<article class="tcard {dragging === card.id ? 'drag' : ''}" draggable="true"
							ondragstart={() => (dragging = card.id ?? null)} ondragend={() => { dragging = null; over = null; }}>
							<a class="plain-link" href="/tracker/{card.id}" style="display: flex; gap: 10px; align-items: center">
								<Logo name={card.company_name ?? ''} url={card.logo_url} size="sm" />
								<span style="min-width: 0">
									<b class="t-heading" style="font-size: 14px; display: block">{card.title}</b>
									<span class="t-caption t-3">{card.company_name}</span>
								</span>
							</a>
							{#if card.job_status === 'closed'}<span class="t-caption" style="color: var(--rejected)">Job closed</span>{/if}
							{#if card.next_step}<span class="t-caption t-2">Next: {card.next_step}</span>{/if}
							<div style="display: flex; justify-content: space-between; align-items: center; gap: 8px">
								<span class="t-caption t-3">{ago(card.updated_at)}</span>
								<select class="move" aria-label="Move {card.title}" value={card.status}
									onchange={(e) => move(card.id ?? 0, e.currentTarget.value as Status)}>
									{#each COLUMNS as c (c.key)}<option value={c.key}>{c.label}</option>{/each}
								</select>
							</div>
						</article>
					{/each}
				</section>
			{/each}
		</div>
	{/if}
</main>

<style>
	.wide { max-width: 1400px; }
	.stats { display: grid; grid-template-columns: repeat(5, 1fr); gap: 10px; }
	.stat { flex-direction: column; align-items: flex-start; gap: 2px; padding: 12px 14px; }
	.tabs { overflow-x: auto; }
	.cols { display: grid; grid-template-columns: 1fr; gap: 12px; }
	.col { display: none; }
	.col.current { display: flex; }
	.col.over { outline: 2px dashed var(--accent); }
	.col-head { padding: 2px 4px; }
	.tcard { cursor: grab; }
	.move { font: 600 12px var(--font); border: 1px solid var(--line); border-radius: 8px; background: var(--surface); color: var(--text-2); padding: 4px 6px; }
	@media (max-width: 600px) { .stats { grid-template-columns: repeat(3, 1fr); } }
	@media (min-width: 900px) {
		.tabs { display: none; }
		.cols { grid-template-columns: repeat(6, minmax(180px, 1fr)); overflow-x: auto; }
		.col { display: flex; }
	}
</style>
