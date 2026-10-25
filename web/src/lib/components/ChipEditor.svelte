<script lang="ts">
	// Editable chips with ×, plus an input to add (S09, S28).
	import Icon from './Icon.svelte';
	let { items = $bindable([]), label, placeholder = 'Add', max = 100 }: {
		items: string[]; label: string; placeholder?: string; max?: number;
	} = $props();
	let text = $state('');
	function add(e: Event) {
		e.preventDefault();
		const v = text.trim();
		if (v && !items.some((i) => i.toLowerCase() === v.toLowerCase()) && items.length < max) items = [...items, v];
		text = '';
	}
</script>

<div class="field">
	<span class="t-caption t-2" style="text-transform: uppercase">{label}</span>
	<div class="chips">
		{#each items as it (it)}
			<span class="skill edit">{it}
				<button type="button" aria-label="Remove {it}" onclick={() => (items = items.filter((x) => x !== it))}
					style="border: 0; background: none; padding: 0; color: var(--text-3); display: inline-flex; cursor: pointer">
					<Icon name="x" size={14} stroke={2.5} />
				</button>
			</span>
		{/each}
	</div>
	<form onsubmit={add} style="display: flex; gap: 8px">
		<input class="input" style="height: 44px" bind:value={text} {placeholder} aria-label="Add to {label}" />
		<button class="btn btn-secondary" style="min-height: 44px">Add</button>
	</form>
</div>
