<script lang="ts">
	import type { Snippet } from 'svelte';
	import Icon from './Icon.svelte';
	let { kind = 'info', children }: { kind?: 'info' | 'warn' | 'err' | 'off' | 'ok'; children: Snippet } = $props();
	const icon = $derived(kind === 'warn' || kind === 'err' ? 'alert' : kind === 'off' ? 'wifiOff' : kind === 'ok' ? 'check' : null);
</script>

<div class="banner b-{kind === 'ok' ? 'info' : kind}" role={kind === 'err' ? 'alert' : 'status'}
	style={kind === 'ok' ? 'background: var(--offer-soft); color: var(--offer)' : ''}>
	{#if icon}<Icon name={icon} size={18} />{/if}
	<span class="txt">{@render children()}</span>
</div>
