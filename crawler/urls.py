"""Canonical URLs and job keys.

The job key is a hash, so a tracking parameter or a trailing slash must never
change it; otherwise one job would look like a new job every night.
"""

import hashlib
import re
from urllib.parse import parse_qsl, urlencode, urlsplit, urlunsplit

# Exact query keys to drop (compared lowercase).
_DROP_KEYS = {
    "ref", "referrer", "source", "src", "gh_src", "lever-source", "lever-origin",
    "gclid", "fbclid", "msclkid", "yclid", "mc_cid", "mc_eid", "_ga", "_gl",
    "sessionid", "session_id", "sid", "jsessionid", "phpsessid", "aspsessionid",
}
# Query key prefixes to drop.
_DROP_PREFIXES = ("utm_",)
# ;jsessionid=... style path parameters.
_PATH_SESSION = re.compile(r";(jsessionid|phpsessid|sid)=[^/?#]*", re.IGNORECASE)
_DEFAULT_PORTS = {"http": 80, "https": 443}


def _keep(key: str) -> bool:
    k = key.lower()
    return k not in _DROP_KEYS and not k.startswith(_DROP_PREFIXES)


def canonicalize(url: str) -> str:
    """Lowercase scheme and host, drop tracking/session params, the fragment,
    default ports and the trailing slash, and sort the remaining params."""
    parts = urlsplit(url.strip())
    scheme = parts.scheme.lower()
    host = (parts.hostname or "").lower()
    if parts.port and parts.port != _DEFAULT_PORTS.get(scheme):
        host = f"{host}:{parts.port}"

    path = _PATH_SESSION.sub("", parts.path).rstrip("/")
    query = urlencode(
        sorted((k, v) for k, v in parse_qsl(parts.query, keep_blank_values=True) if _keep(k))
    )
    return urlunsplit((scheme, host, path, query, ""))


def job_key(company_id: int, ats: str | None, source_job_id: str | None, url: str | None) -> str:
    """sha256 hex of company + the ATS job ID, or of company + canonical job URL."""
    if source_job_id:
        material = f"{company_id}|{ats or ''}|id|{source_job_id}"
    elif url:
        material = f"{company_id}|url|{canonicalize(url)}"
    else:
        raise ValueError("a job needs an ATS job ID or a URL")
    return hashlib.sha256(material.encode()).hexdigest()
