<script lang="ts">
	import { scoreBand } from '$lib/format';
	let { score, size = 'md' }: { score: number | null | undefined; size?: 'sm' | 'md' | 'lg' } = $props();
</script>

<div
	class="ring {size === 'md' ? '' : size} {scoreBand(score)}"
	style="--p: {score ?? 0}%"
	role="img"
	aria-label={score == null ? 'No score' : `Match score ${score} out of 100`}
>
	<span>{score ?? '—'}</span>
</div>
