<script lang="ts">
	import { initials, logoClass } from '$lib/format';
	let { name, url = null, size = 'md' }: { name: string; url?: string | null; size?: 'sm' | 'md' | 'lg' } = $props();
</script>

{#if url}
	<img class="logo {size === 'md' ? '' : size}" src={url} alt="" style="object-fit: contain; background: var(--surface)" />
{:else}
	<span class="logo {size === 'md' ? '' : size} {logoClass(name)}" aria-hidden="true">{initials(name)}</span>
{/if}
