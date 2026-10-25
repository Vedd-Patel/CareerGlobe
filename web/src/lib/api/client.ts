// Typed client for the CareerGlobe API (types generated from api/openapi.yaml).
import { API_URL } from '$lib/config';
import type { components } from './schema';

export type Schemas = components['schemas'];

/** An error the API answered with: `code` is stable, `message` is for people. */
export class ApiError extends Error {
	constructor(
		public status: number,
		public code: string,
		message: string
	) {
		super(message);
	}
}

export type TokenSource = () => Promise<string | null>;
let tokenSource: TokenSource = async () => null;

/** The auth module registers how to get the current ID token. */
export function setTokenSource(fn: TokenSource): void {
	tokenSource = fn;
}

type Query = Record<string, string | number | boolean | undefined | null>;

export function buildUrl(path: string, query?: Query): string {
	const url = new URL(API_URL + path);
	for (const [k, v] of Object.entries(query ?? {})) {
		if (v === undefined || v === null || v === '' || v === false) continue;
		url.searchParams.set(k, v === true ? '1' : String(v));
	}
	return url.toString();
}

export async function request<T>(
	method: string,
	path: string,
	opts: { query?: Query; body?: unknown; form?: FormData; auth?: boolean; deviceId?: string } = {}
): Promise<T> {
	const headers: Record<string, string> = { Accept: 'application/json' };
	if (opts.auth !== false) {
		const token = await tokenSource();
		if (token) headers.Authorization = `Bearer ${token}`;
	}
	if (opts.deviceId) headers['X-Device-Id'] = opts.deviceId;
	let body: BodyInit | undefined;
	if (opts.form) body = opts.form;
	else if (opts.body !== undefined) {
		headers['Content-Type'] = 'application/json';
		body = JSON.stringify(opts.body);
	}
	let res: Response;
	try {
		res = await fetch(buildUrl(path, opts.query), { method, headers, body });
	} catch {
		throw new ApiError(0, 'offline', "You're offline or the server can't be reached.");
	}
	const text = await res.text();
	const data = text ? safeJson(text) : null;
	if (!res.ok) {
		const err = (data ?? {}) as { error?: string; message?: string };
		throw new ApiError(res.status, err.error ?? 'http_' + res.status, err.message ?? 'Something went wrong.');
	}
	return data as T;
}

function safeJson(text: string): unknown {
	try {
		return JSON.parse(text);
	} catch {
		return null;
	}
}

export const api = {
	get: <T>(path: string, query?: Query) => request<T>('GET', path, { query }),
	post: <T>(path: string, body?: unknown) => request<T>('POST', path, { body }),
	put: <T>(path: string, body?: unknown) => request<T>('PUT', path, { body }),
	patch: <T>(path: string, body?: unknown) => request<T>('PATCH', path, { body }),
	del: <T>(path: string, body?: unknown) => request<T>('DELETE', path, { body })
};
