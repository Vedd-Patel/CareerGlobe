<script lang="ts">
	import Icon from './Icon.svelte';
	import ScoreRing from './ScoreRing.svelte';
	import { ago, workMode } from '$lib/format';
	let { job, onSave }: {
		job: {
			job_id: number; title: string; company_name?: string; cities?: string[]; work_mode?: string | null;
			first_seen?: string; score?: number | null; reason?: string; new?: boolean; saved?: boolean; closed?: boolean;
		};
		onSave?: (id: number) => void;
	} = $props();
	const where = $derived([...(job.cities ?? []).slice(0, 2), workMode(job.work_mode)].filter(Boolean).join(' · ') || 'India');
</script>

<article class="card {job.closed ? 'closed-card' : ''}" style="align-items: flex-start">
	<ScoreRing score={job.score} size="sm" />
	<a href="/jobs/{job.job_id}" style="flex: 1; min-width: 0; color: inherit; text-decoration: none; display: flex; flex-direction: column; gap: 4px">
		<div style="display: flex; gap: 8px; align-items: center; flex-wrap: wrap">
			<h3 class="t-heading" style="font-size: 16px">{job.title}</h3>
			{#if job.new}<span class="badge-new">NEW</span>{/if}
		</div>
		<p class="t-caption t-2">{[job.company_name, where, ago(job.first_seen)].filter(Boolean).join(' · ')}</p>
		{#if job.reason}<p class="t-caption t-3">{job.reason}</p>{/if}
	</a>
	{#if onSave}
		<button class="icon-btn" aria-label={job.saved ? 'Saved' : 'Save job'} aria-pressed={job.saved}
			style={job.saved ? 'background: var(--accent); color: var(--on-accent); border-color: var(--accent)' : ''}
			onclick={() => onSave?.(job.job_id)}>
			<Icon name="bookmark" fill={job.saved} />
		</button>
	{/if}
</article>
