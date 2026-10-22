package server

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The OpenAPI file is what the Android and iOS clients are generated from:
// every route the server registers must be in it, and nothing else.
func TestOpenAPIListsEveryRoute(t *testing.T) {
	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	spec, err := os.ReadFile(filepath.Join("..", "..", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	routeRe := regexp.MustCompile(`"(GET|POST|PUT|PATCH|DELETE) (/[^"]*)"`)
	var routes []string
	for _, m := range routeRe.FindAllStringSubmatch(string(src), -1) {
		routes = append(routes, strings.ToLower(m[1])+" "+m[2])
	}

	var documented []string
	path := ""
	pathRe := regexp.MustCompile(`^  (/\S*):\s*$`)
	methodRe := regexp.MustCompile(`^    (get|post|put|patch|delete):\s*$`)
	for _, line := range strings.Split(string(spec), "\n") {
		if m := pathRe.FindStringSubmatch(line); m != nil {
			path = m[1]
		} else if m := methodRe.FindStringSubmatch(line); m != nil && path != "" {
			documented = append(documented, m[1]+" "+path)
		} else if strings.HasPrefix(line, "components:") {
			break
		}
	}
	sort.Strings(routes)
	sort.Strings(documented)
	if strings.Join(routes, "\n") != strings.Join(documented, "\n") {
		missing, extra := diff(routes, documented), diff(documented, routes)
		t.Fatalf("openapi.yaml is out of date.\nnot documented: %v\ndocumented but not served: %v", missing, extra)
	}
	if len(routes) < 50 {
		t.Fatalf("only %d routes found; the parser is broken", len(routes))
	}
}

func diff(a, b []string) []string {
	in := map[string]bool{}
	for _, v := range b {
		in[v] = true
	}
	var out []string
	for _, v := range a {
		if !in[v] {
			out = append(out, v)
		}
	}
	return out
}
