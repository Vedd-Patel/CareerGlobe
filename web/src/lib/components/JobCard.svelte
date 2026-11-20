<script lang="ts">
	import Icon from './Icon.svelte';
	import Logo from './Logo.svelte';
	import ScoreRing from './ScoreRing.svelte';
	import { ago, employmentLabel, salary, workMode, yearsLabel } from '$lib/format';
	let { job, onSave }: {
		job: {
			job_id: number; title: string; company_name?: string; logo_url?: string | null; cities?: string[]; work_mode?: string | null;
			first_seen?: string; score?: number | null; reason?: string; new?: boolean; saved?: boolean; closed?: boolean;
			employment_type?: string | null; years_min?: number | null; years_max?: number | null;
			salary_min?: number | null; salary_max?: number | null;
		};
		onSave?: (id: number) => void;
	} = $props();
	const where = $derived([...(job.cities ?? []).slice(0, 2), workMode(job.work_mode)].filter(Boolean).join(' · ') || 'India');
	// Small facts about the job, shown as tags: type (when not plain full-time), experience, pay.
	const tags = $derived(
		[employmentLabel(job.employment_type), yearsLabel(job.years_min, job.years_max), salary(job.salary_min ?? null, job.salary_max ?? null)].filter(Boolean)
	);
</script>

<article class="card lift job {job.closed ? 'closed-card' : ''}" style="align-items: flex-start">
	<ScoreRing score={job.score} size="sm" />
	<a href="/jobs/{job.job_id}" style="flex: 1; min-width: 0; color: inherit; text-decoration: none; display: flex; flex-direction: column; gap: 6px">
		<div style="display: flex; gap: 8px; align-items: center; flex-wrap: wrap">
			<h3 class="t-heading" style="font-size: 16px">{job.title}</h3>
			{#if job.new}<span class="badge-new">NEW</span>{/if}
		</div>
		<p class="t-caption t-2 byline">
			{#if job.company_name}<Logo name={job.company_name} url={job.logo_url ?? null} size="xs" />{/if}
			<span>{[job.company_name, where, ago(job.first_seen)].filter(Boolean).join(' · ')}</span>
		</p>
		{#if tags.length}
			<div class="tags">{#each tags as t (t)}<span class="tag">{t}</span>{/each}</div>
		{/if}
		{#if job.reason}<p class="t-caption t-3">{job.reason}</p>{/if}
	</a>
	{#if onSave}
		<button class="icon-btn" aria-label={job.saved ? 'Saved' : 'Save job'} aria-pressed={job.saved}
			style={job.saved ? 'background: var(--accent-soft); color: var(--accent); border-color: transparent' : ''}
			onclick={() => onSave?.(job.job_id)}>
			<Icon name="bookmark" fill={job.saved} />
		</button>
	{/if}
</article>

<style>
	.byline { display: flex; align-items: center; gap: 8px; min-width: 0; }
	.byline span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.tags { display: flex; flex-wrap: wrap; gap: 6px; }
	.tag {
		font: 650 11px/16px var(--font); letter-spacing: 0.02em; padding: 2px 8px; border-radius: 6px;
		background: var(--raised); color: var(--text-2); border: 1px solid color-mix(in srgb, var(--line) 70%, transparent);
	}
</style>
