// The parsed resume handed from S08 to S09.
import type { Schemas } from '$lib/api/client';
export const draft = $state<{ value: Schemas['ResumeDraft'] | null }>({ value: null });
