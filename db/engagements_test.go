package db

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"
)

func TestNormalizeEngagementScope(t *testing.T) {
	ok := []struct{ kind, in, want string }{
		{"domain", "https://Admin.Example.com:8443/login?x=1", "admin.example.com"},
		{"domain", "*.example.com.", "example.com"},
		{"ip", "10.0.0.1", "10.0.0.1"},
		{"cidr", "10.1.2.3/24", "10.1.2.0/24"},
		{"keyword", "  Acme   Corp ", "acme corp"},
	}
	for _, tc := range ok {
		got, err := NormalizeEngagementScope(tc.kind, tc.in)
		if tc.want == "" {
			if err == nil {
				t.Errorf("%s %q should be invalid, got %q", tc.kind, tc.in, got)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("%s %q = %q, %v; want %q", tc.kind, tc.in, got, err, tc.want)
		}
	}
	for _, bad := range [][2]string{{"domain", "not a domain"}, {"domain", "localhost"}, {"ip", "999.1.1.1"}, {"cidr", "10.0.0.0/33"}, {"domain", ""}, {"icp", "x"}} {
		if _, err := NormalizeEngagementScope(bad[0], bad[1]); err == nil {
			t.Errorf("%v should be rejected", bad)
		}
	}
}

// MatchAssetInterceptRulesForTest reports whether any rule blocks the host (test helper).
func MatchAssetInterceptRulesForTest(rules []AssetInterceptRule, host string) bool {
	_, _, hit := MatchAssetInterceptRules(rules, []string{host}, nil, nil)
	return hit
}

func TestEngagementValidate(t *testing.T) {
	e := &Engagement{Name: " Acme external "}
	if err := e.Validate(); err != nil || e.Name != "Acme external" || e.Status != "scoping" || e.BudgetAction != "alert" {
		t.Fatalf("defaults: %v %+v", err, e)
	}
	bad := []*Engagement{
		{Name: ""},
		{Name: "x", Status: "bogus"},
		{Name: "x", BudgetAction: "explode"},
		{Name: "x", WindowEnforce: true},
		{Name: "x", WindowTZ: "Mars/Base"},
		{Name: "x", DailyWindows: json.RawMessage(`[{"start":"9:00","end":"9:00"}]`)},
		{Name: "x", BudgetAlertPct: []int{0}},
	}
	for i, b := range bad {
		if err := b.Validate(); err == nil {
			t.Errorf("case %d should be invalid", i)
		}
	}
}

func TestEngagementLifecyclePG(t *testing.T) {
	d := openTargetDB(t)
	name := "pg-test-engagement-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	budget := 12.5
	start := time.Now().Add(-time.Hour)
	end := time.Now().Add(time.Hour)
	id, err := d.SaveEngagement(&Engagement{Name: name, Status: "testing", BudgetEUR: &budget, BudgetAlertPct: []int{100, 50},
		WindowStart: &start, WindowEnd: &end, WindowEnforce: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.DeleteEngagement(id) })
	if _, err := d.SaveEngagement(&Engagement{Name: name}); err != ErrEngagementExists {
		t.Fatalf("duplicate name: %v", err)
	}
	got, err := d.GetEngagement(id)
	if err != nil || got.BudgetEUR == nil || *got.BudgetEUR != 12.5 || len(got.BudgetAlertPct) != 2 || got.BudgetAlertPct[0] != 50 {
		t.Fatalf("round trip: %v %+v", err, got)
	}

	// scope + exclusion rules
	if added, _, err := d.AddEngagementScope(id, "domain", "Prod.Example.com", true, "payments"); err != nil || !added {
		t.Fatal(added, err)
	}
	if added, _, _ := d.AddEngagementScope(id, "domain", "prod.example.com", true, ""); added {
		t.Error("duplicate scope entry must not be added twice")
	}
	if _, _, err := d.AddEngagementScope(id, "keyword", "acme", true, ""); err == nil {
		t.Error("an excluded keyword must be rejected")
	}
	if _, _, err := d.AddEngagementScope(id, "cidr", "10.9.0.0/16", true, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := d.AddEngagementScope(id, "domain", "example.com", false, ""); err != nil {
		t.Fatal(err)
	}
	rules, err := d.EngagementExclusionRules(id)
	if err != nil || len(rules) != 3 { // exact + fuzzy for the domain, one cidr; the in-scope entry is not a rule
		t.Fatalf("rules = %d %v", len(rules), err)
	}
	if hit := MatchAssetInterceptRulesForTest(rules, "api.prod.example.com"); !hit {
		t.Error("a subdomain of an excluded domain must match")
	}
	if hit := MatchAssetInterceptRulesForTest(rules, "example.com"); hit {
		t.Error("the in-scope parent domain must not match")
	}

	// events: dedup
	if ins, err := d.AddEngagementEvent(id, "", "budget_alert", map[string]any{"pct": 80}, "budget:80"); err != nil || !ins {
		t.Fatal(ins, err)
	}
	if ins, _ := d.AddEngagementEvent(id, "", "budget_alert", map[string]any{"pct": 80}, "budget:80"); ins {
		t.Error("deduplicated event written twice")
	}
	if evs, _ := d.ListEngagementEvents(id, 10); len(evs) != 1 {
		t.Errorf("events = %d", len(evs))
	}

	// deleting the engagement detaches, never deletes, tasks
	task, err := d.CreateTaskWithOptions("eng task", "goal", TaskCreateOptions{})
	if err != nil {
		t.Skipf("cannot create a task here: %v", err)
	}
	if err := d.SetTaskEngagement(task.ID, &id); err != nil {
		t.Fatal(err)
	}
	if ids, _ := d.EngagementTaskIDs(id); len(ids) != 1 || ids[0] != task.ID {
		t.Fatalf("task ids = %v", ids)
	}
	if closed, _, err := d.TaskWindowClosed(task.ID, time.Now()); err != nil || closed {
		t.Fatalf("inside the window: closed=%v err=%v", closed, err)
	}
	if closed, reason, _ := d.TaskWindowClosed(task.ID, end.Add(time.Minute)); !closed || reason == "" {
		t.Fatalf("after the window: closed=%v reason=%q", closed, reason)
	}
	if err := d.DeleteEngagement(id); err != nil {
		t.Fatal(err)
	}
	if e, _ := d.EngagementForTask(task.ID); e != nil {
		t.Error("task still attached after the engagement was deleted")
	}
	if tt, err := d.GetTask(task.ID); err != nil || tt == nil {
		t.Error("deleting an engagement must not delete its tasks")
	}
}
