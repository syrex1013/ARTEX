package classify

import (
	"slices"
	"testing"
)

func TestNormalize(t *testing.T) {
	for _, tc := range []struct {
		kind, in, want string
		ok             bool
	}{
		{"cwe", "79", "CWE-79", true},
		{"cwe", " cwe-89 ", "CWE-89", true},
		{"cwe", "CWE79", "CWE-79", true},
		{"cwe", "CWE-", "", false},
		{"cwe", "abc", "", false},
		{"owasp", "a03:2021", "A03:2021", true},
		{"owasp", "A11:2021", "", false},
		{"attack", "t1190", "T1190", true},
		{"attack", "T1110.003", "T1110.003", true},
		{"attack", "T11", "", false},
	} {
		got, ok := Normalize(tc.kind, tc.in)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("Normalize(%s,%q) = %q,%v want %q,%v", tc.kind, tc.in, got, ok, tc.want, tc.ok)
		}
	}
	if got, bad := NormalizeList("cwe", []string{"79", "CWE-79", "", "89"}); bad != "" || !slices.Equal(got, []string{"CWE-79", "CWE-89"}) {
		t.Errorf("NormalizeList = %v %q", got, bad)
	}
	if _, bad := NormalizeList("cwe", []string{"79", "nope"}); bad != "nope" {
		t.Errorf("invalid id not reported: %q", bad)
	}
}

func TestListsAreWellFormedAndUnique(t *testing.T) {
	for kind, list := range map[string][]Entry{"cwe": CWE, "owasp": OWASP, "attack": Attack} {
		seen := map[string]bool{}
		for _, e := range list {
			if _, ok := Normalize(kind, e.ID); !ok {
				t.Errorf("%s: %q is malformed", kind, e.ID)
			}
			if e.Name == "" || seen[e.ID] {
				t.Errorf("%s: %q has no name or is duplicated", kind, e.ID)
			}
			seen[e.ID] = true
			if kind == "attack" && e.Tactic == "" {
				t.Errorf("attack %s has no tactic", e.ID)
			}
		}
	}
}

func TestEverySuggestedIDExists(t *testing.T) {
	for _, r := range rules {
		for kind, ids := range map[string][]string{"cwe": r.cwe, "owasp": r.owasp, "attack": r.attack} {
			for _, id := range ids {
				if Lookup(kind, id).Name == "" {
					t.Errorf("rule %q suggests %s %s which is not in the curated list", r.label, kind, id)
				}
			}
		}
	}
}

func TestSuggest(t *testing.T) {
	s := Suggest("SQL Injection", "Login form SQL injection leading to RCE")
	for _, want := range []string{"CWE-89", "CWE-78"} {
		if !slices.Contains(s.CWE, want) {
			t.Errorf("missing %s in %v", want, s.CWE)
		}
	}
	if !slices.Contains(s.OWASP, "A03:2021") || !slices.Contains(s.Attack, "T1190") || s.Reason == "" {
		t.Errorf("suggestion = %+v", s)
	}
	none := Suggest("something unrelated entirely")
	if len(none.CWE)+len(none.OWASP)+len(none.Attack) != 0 || none.CWE == nil {
		t.Errorf("no match must give empty non-nil lists: %+v", none)
	}
}
