package skills

import (
	"slices"
	"testing"
)

func TestExtract(t *testing.T) {
	e := Default()
	text := `Senior engineer. Skills: Java, Go, Python | React Native, ReactJS, Node.js, k8s, AWS
	Built RTL in SystemVerilog and UVM; wrote Embedded C for ARM Cortex. Figma prototyping. C++ and C#.
	Go to market planning. Contact me at 5 PM. CV attached. Based in Hyderabad, TS.`
	got := e.Extract(text)
	for _, want := range []string{"Java", "Go", "Python", "React Native", "React", "Node.js", "Kubernetes", "AWS",
		"RTL Design", "SystemVerilog", "UVM", "Embedded C", "ARM", "Figma", "Prototyping", "C++", "C#"} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %q in %v", want, got)
		}
	}
	for _, bad := range []string{"Computer Vision", "Product Management", "TypeScript", "C", "R"} {
		if slices.Contains(got, bad) {
			t.Errorf("false positive %q in %v", bad, got)
		}
	}
}

func TestGoOnlyInLists(t *testing.T) {
	e := Default()
	if slices.Contains(e.Extract("Go ahead and apply. We go fast."), "Go") {
		t.Fatal("Go matched in prose")
	}
	if !slices.Contains(e.Extract("Languages: Go / Rust"), "Go") || !slices.Contains(e.Extract("Golang"), "Go") {
		t.Fatal("Go not found in a list or as golang")
	}
}

func TestUpperOnlyAndNoDuplicates(t *testing.T) {
	e := Default()
	if got := e.Extract("ml engineer; we can help"); slices.Contains(got, "Machine Learning") || slices.Contains(got, "CAN") {
		t.Fatalf("lowercase short words matched: %v", got)
	}
	got := e.Extract("ML, Machine Learning, machine learning")
	if n := len(slices.DeleteFunc(slices.Clone(got), func(s string) bool { return s != "Machine Learning" })); n != 1 {
		t.Fatalf("duplicates: %v", got)
	}
}

func TestExtraSynonymsAndCanonical(t *testing.T) {
	e := New(map[string]string{"springboot2": "Spring Boot"})
	if !slices.Contains(e.Extract("SpringBoot2 services"), "Spring Boot") {
		t.Fatal("extra synonym not used")
	}
	if got := Default().Canonical("reactjs"); got != "React" {
		t.Fatalf("Canonical = %q", got)
	}
	if got := Default().Canonical("  Basket Weaving "); got != "Basket Weaving" {
		t.Fatalf("Canonical unknown = %q", got)
	}
}
