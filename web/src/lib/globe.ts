// Filter state for the globe and matches (S14 chips, S16 sheet) -> API query.
export type Filters = {
	category: string;
	remote: boolean;
	type: '' | 'product' | 'service';
	posted: '' | '24h' | '7d' | '30d';
	hiring: boolean;
	matches: boolean;
	employment: string[]; // EMPLOYMENT ids; empty = any type
	exp: string; // EXPERIENCE id; '' = any
	minSalaryLpa: number; // lakh rupees a year; 0 = any
};

export const EMPLOYMENT = [
	{ id: 'intern', label: 'Internship' },
	{ id: 'full_time', label: 'Full-time' },
	{ id: 'part_time', label: 'Part-time' },
	{ id: 'contract', label: 'Contract' }
] as const;

/** Years of experience bands: a job passes when the years it asks for meet the band (or it asks for none). */
export const EXPERIENCE = [
	{ id: 'fresher', label: 'Fresher', hint: '0–1 yr', min: 0, max: 1 },
	{ id: '1-3', label: '1–3 years', hint: '', min: 1, max: 3 },
	{ id: '3-5', label: '3–5 years', hint: '', min: 3, max: 5 },
	{ id: '5-8', label: '5–8 years', hint: '', min: 5, max: 8 },
	{ id: '8+', label: '8+ years', hint: '', min: 8, max: 60 }
] as const;

export const SALARY_LPA = [5, 10, 15, 25, 40] as const;

export const emptyFilters = (): Filters => ({
	category: '', remote: false, type: '', posted: '', hiring: false, matches: false, employment: [], exp: '', minSalaryLpa: 0
});

/** The job-level filters (type, experience, salary) as API query parameters; off ones are ''. */
export function jobTypeQuery(f: Pick<Filters, 'employment' | 'exp' | 'minSalaryLpa'>): Record<string, string> {
	const band = EXPERIENCE.find((b) => b.id === f.exp);
	return {
		employment: f.employment.join(','),
		exp_min: band ? String(band.min) : '',
		exp_max: band ? String(band.max) : '',
		min_salary: f.minSalaryLpa > 0 ? String(f.minSalaryLpa * 100_000) : ''
	};
}

export function filtersToQuery(f: Filters): Record<string, string | boolean> {
	return { category: f.category, remote: f.remote, type: f.type, posted: f.posted, hiring: f.hiring, matches: f.matches, ...jobTypeQuery(f) };
}

export function activeCount(f: Filters): number {
	return [f.category, f.remote, f.type, f.posted, f.hiring, f.matches, f.employment.length > 0, f.exp, f.minSalaryLpa > 0].filter(Boolean).length;
}

/** Cluster bubble size class from its company count. */
export function clusterSize(n: number): 'c-s' | 'c-m' | 'c-l' {
	return n >= 100 ? 'c-l' : n >= 10 ? 'c-m' : 'c-s';
}

/** Pin class: matching jobs > hiring > not hiring. */
export function pinKind(p: { matches?: number; open_jobs?: number }): 'match' | 'hiring' | 'idle' {
	return (p.matches ?? 0) > 0 ? 'match' : (p.open_jobs ?? 0) > 0 ? 'hiring' : 'idle';
}
