<script lang="ts">
	let { count = 4 }: { count?: number } = $props();
</script>

<div style="display: flex; flex-direction: column; gap: 12px" aria-busy="true" aria-label="Loading">
	{#each Array(count) as _, i (i)}
		<div class="card" style="gap: 12px">
			<span class="sk" style="width: 44px; height: 44px; border-radius: 50%"></span>
			<div style="flex: 1; display: flex; flex-direction: column; gap: 8px">
				<span class="sk" style="height: 16px; width: 72%"></span>
				<span class="sk" style="height: 12px; width: 48%"></span>
				<span class="sk" style="height: 12px; width: 88%"></span>
			</div>
		</div>
	{/each}
</div>
