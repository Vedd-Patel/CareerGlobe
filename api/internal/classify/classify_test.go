package classify

import "testing"

func TestCategory(t *testing.T) {
	c := New(nil)
	tests := []struct{ title, first, second string }{
		{"Senior Backend Engineer", "Software Engineering", ""},
		{"Embedded Software Engineer", "Hardware & Semiconductor", "Software Engineering"},
		{"Product Designer", "Design", ""},
		{"Senior Product Manager", "Product", ""},
		{"ASIC Design Engineer - Performance Monitoring", "Hardware & Semiconductor", ""},
		{"Data Scientist II", "Data & AI", ""},
		{"SDET", "QA & Testing", ""},
		{"Site Reliability Engineer", "DevOps & IT", ""},
		{"Talent Acquisition Partner", "Recruiting/HR", ""},
		{"Account Executive, Mid-Market", "Sales", ""},
		{"Growth Marketing Lead", "Marketing", ""},
		{"Senior Manager- Warehousing operations", "Operations", ""},
		{"Assistant Manager - Internal Audit", "Finance", ""},
		{"Customer Success Account Manager", "Customer Support", ""},
		{"Corporate Counsel", "Legal", ""},
		{"Chef de Partie", "Other", ""},
		{"Detail-oriented Designer", "Design", ""}, // "ai" inside "Detail" must not match
		{"Applied Scientist II, Buyer Risk Prevention (BRP)", "Data & AI", "Finance"},
		{"Account Manager / Senior Account Manager", "Sales", ""},
		{"Specialist - F&A - AP 4B", "Finance", ""},
		{"Tech Lead - Enterprise Application", "Software Engineering", ""},
		{"Executive - Production", "Operations", ""},
		{"DCO Tech 3, HYD-Infinity - DCO", "DevOps & IT", ""},
		{"Senior Mechanical Design Engineer", "Other", ""},
		{"QAE II, Amazon Payments", "QA & Testing", ""},
	}
	for _, tt := range tests {
		first, second := c.Category(tt.title)
		if first != tt.first || second != tt.second {
			t.Errorf("Category(%q) = %q, %q; want %q, %q", tt.title, first, second, tt.first, tt.second)
		}
	}
}

func TestAdminRulesWin(t *testing.T) {
	c := New([]Rule{{Keyword: "chef", Category: "Operations", Priority: 1}})
	if got, _ := c.Category("Chef de Partie"); got != "Operations" {
		t.Fatalf("got %q", got)
	}
}

func TestSeniority(t *testing.T) {
	tests := map[string]string{
		"Software Engineering Intern":      "Intern",
		"Graduate Engineer Trainee":        "Intern",
		"Fresher - Customer Support":       "Fresher",
		"Junior Developer":                 "Junior",
		"SDE 1":                            "Junior",
		"Software Engineer II":             "Mid",
		"Backend Engineer":                 "Mid",
		"Senior Software Engineer":         "Senior",
		"Staff Engineer":                   "Senior",
		"Principal Engineer":               "Lead+",
		"Engineering Manager":              "Lead+",
		"Senior Product Manager":           "Senior",
		"Customer Success Account Manager": "Mid",
	}
	for title, want := range tests {
		if got := Seniority(title); got != want {
			t.Errorf("Seniority(%q) = %q, want %q", title, got, want)
		}
	}
}

func TestBandForYears(t *testing.T) {
	for years, want := range map[float64]string{0: "Fresher", 0.5: "Fresher", 2: "Junior", 4: "Mid", 7: "Senior", 12: "Lead+"} {
		if got := BandForYears(years); got != want {
			t.Errorf("BandForYears(%v) = %q, want %q", years, got, want)
		}
	}
}
