<script lang="ts">
	let { status }: { status: string } = $props();
	const label = $derived(status.charAt(0).toUpperCase() + status.slice(1));
</script>

<span class="pill p-{status}">{label}</span>
