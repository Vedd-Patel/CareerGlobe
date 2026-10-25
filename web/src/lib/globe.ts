// Filter state for the globe and matches (S14 chips, S16 sheet) -> API query.
export type Filters = {
	category: string;
	remote: boolean;
	type: '' | 'product' | 'service';
	posted: '' | '24h' | '7d' | '30d';
	hiring: boolean;
	matches: boolean;
};

export const emptyFilters = (): Filters => ({ category: '', remote: false, type: '', posted: '', hiring: false, matches: false });

export function filtersToQuery(f: Filters): Record<string, string | boolean> {
	return { category: f.category, remote: f.remote, type: f.type, posted: f.posted, hiring: f.hiring, matches: f.matches };
}

export function activeCount(f: Filters): number {
	return [f.category, f.remote, f.type, f.posted, f.hiring, f.matches].filter(Boolean).length;
}

/** Cluster bubble size class from its company count. */
export function clusterSize(n: number): 'c-s' | 'c-m' | 'c-l' {
	return n >= 100 ? 'c-l' : n >= 10 ? 'c-m' : 'c-s';
}

/** Pin class: matching jobs > hiring > not hiring. */
export function pinKind(p: { matches?: number; open_jobs?: number }): 'match' | 'hiring' | 'idle' {
	return (p.matches ?? 0) > 0 ? 'match' : (p.open_jobs ?? 0) > 0 ? 'hiring' : 'idle';
}
