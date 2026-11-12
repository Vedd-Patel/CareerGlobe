package server

import (
	"net/netip"
	"net/url"
	"strings"
)

// publicURL reports whether raw is an http(s) URL on a public host name. The
// crawler fetches careers URLs, so ones that point at this machine or a
// private network (localhost, 10.x, 169.254.x, *.internal, bare IPs) are refused.
func publicURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "" || !strings.Contains(host, ".") || len(raw) > 2000 {
		return false
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return false // company sites have names; an IP literal is never one
	}
	for _, suffix := range []string{".local", ".localhost", ".internal", ".lan", ".home", ".corp", ".test", ".invalid"} {
		if strings.HasSuffix(host, suffix) {
			return false
		}
	}
	return true
}
