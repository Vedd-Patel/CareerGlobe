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

func TestEmployment(t *testing.T) {
	for _, c := range []struct{ title, desc, want string }{
		{"Software Engineering Intern", "", "intern"},
		{"Graduate Trainee", "", "intern"},
		{"Data Analyst (Contract)", "", "contract"},
		{"Associate Contract Manager", "", "full_time"},
		{"Consultant - Cloud", "", "full_time"}, // a consulting job title, not a contract
		{"Support Associate - Part Time", "", "part_time"},
		{"Backend Engineer", "Employment type: Contract. Work with us.", "contract"},
		{"Backend Engineer", "We run an internship programme and hire full time.", "full_time"},
		{"Backend Engineer", "", "full_time"},
	} {
		if got := Employment(c.title, c.desc); got != c.want {
			t.Errorf("%q / %q: got %s, want %s", c.title, c.desc, got, c.want)
		}
	}
}

func TestYearsRange(t *testing.T) {
	i := func(n int16) *int16 { return &n }
	for _, c := range []struct {
		title, desc string
		min, max    *int16
	}{
		{"Engineer", "Requires 3-5 years of experience in Go.", i(3), i(5)},
		{"Engineer", "3 to 5 yrs experience", i(3), i(5)},
		{"Engineer", "You bring 5+ years of relevant experience.", i(5), nil},
		{"Engineer", "Minimum 2 years experience with SQL", i(2), nil},
		{"Engineer", "Experience: 4 years", i(4), nil},
		{"Engineer", "We were founded 10 years ago and love pizza.", nil, nil},
		{"Engineer", "Salary range 40-60 lakhs", nil, nil},
		{"Engineer", "Freshers are welcome to apply", i(0), i(1)},
		{"Software Intern", "", i(0), i(1)},
		{"Engineer", "", nil, nil},
	} {
		min, max := YearsRange(c.title, c.desc)
		if !same(min, c.min) || !same(max, c.max) {
			t.Errorf("%q / %q: got %v-%v, want %v-%v", c.title, c.desc, show(min), show(max), show(c.min), show(c.max))
		}
	}
}

func same(a, b *int16) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }

func show(p *int16) any {
	if p == nil {
		return "nil"
	}
	return *p
}
