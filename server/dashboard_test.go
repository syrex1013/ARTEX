package server

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Autumn-27/artex/db"
)

func TestDashboardUsageCountsCompletedRunsOnceAcrossAllTasks(t *testing.T) {
	s, _ := collaborationFixture(t)
	_, token := collaborationUser(t, s, "dashboard-admin", true)
	task, err := s.m.pg.CreateTask("Usage regression", "Verify dashboard counts", nil, 0, 300)
	if err != nil {
		t.Fatal(err)
	}
	input, output, cached := 100, 20, 80
	for _, kind := range []string{"usage", "usage", "result"} {
		if _, err := s.m.pg.Exploration(task.ExplorationID).AppendActivity(db.Activity{
			Worker: "mainagent", Kind: kind, InputTokens: &input, OutputTokens: &output, CacheReadTokens: &cached,
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Usage must survive exclusion of this task from the 20-row recent list.
	for i := 0; i < 21; i++ {
		if _, err := s.m.pg.CreateTask("Recent task", "Dashboard fixture", nil, 0, 300); err != nil {
			t.Fatal(err)
		}
	}
	conv, err := s.m.pg.CreateConversation("mainagent", "Usage conversation", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.m.pg.AppendConvActivity(conv.ID, db.Activity{Kind: "result", InputTokens: &input, OutputTokens: &output, CacheReadTokens: &cached}); err != nil {
		t.Fatal(err)
	}
	w := collaborationRequest(s, token, http.MethodGet, "/api/dashboard/tasks", "")
	var got struct {
		Total int `json:"total"`
		Tasks []struct {
			ID string `json:"id"`
		} `json:"tasks"`
		TokenProfiles []db.TokenUsage `json:"token_profiles"`
		TokenDaily    []struct {
			Date string `json:"date"`
			db.TokenUsage
		} `json:"token_daily"`
	}
	if w.Code != http.StatusOK {
		t.Fatalf("dashboard status=%d", w.Code)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Total != 22 || len(got.Tasks) != 20 || len(got.TokenProfiles) != 1 || got.TokenProfiles[0].InputTokens != 100 || got.TokenProfiles[0].OutputTokens != 20 || got.TokenProfiles[0].CacheReadTokens != 80 {
		t.Fatalf("dashboard totals=%+v", got)
	}
	var dailyInput, dailyOutput, dailyCached int
	for _, day := range got.TokenDaily {
		if day.Date != time.Now().UTC().Format("2006-01-02") {
			t.Fatalf("unexpected UTC date %q", day.Date)
		}
		dailyInput += day.InputTokens
		dailyOutput += day.OutputTokens
		dailyCached += day.CacheReadTokens
	}
	if dailyInput != 200 || dailyOutput != 40 || dailyCached != 160 {
		t.Fatalf("daily usage=%d/%d/%d, want task plus conversation results only", dailyInput, dailyOutput, dailyCached)
	}
}
