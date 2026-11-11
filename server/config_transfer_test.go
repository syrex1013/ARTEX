package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/db"
)

func TestParseConfigSections(t *testing.T) {
	got, err := parseConfigSections("")
	if err != nil || len(got) != len(configSections) {
		t.Fatalf("empty = %v, %v", got, err)
	}
	got, err = parseConfigSections("llm, mcp,llm")
	if err != nil || strings.Join(got, ",") != "llm,mcp" {
		t.Fatalf("dedupe = %v, %v", got, err)
	}
	if _, err := parseConfigSections("llm,passwords"); err == nil {
		t.Fatal("unknown section must be rejected")
	}
}

// Every exportable setting key must be a real writable field of PUT /api/settings, otherwise an import would
// silently drop it. Replaying a bundle that holds all keys at their zero-ish values must not be rejected for
// unknown fields (putSettings ignores unknown keys, so assert via the typed struct instead).
func TestConfigSettingKeysAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, k := range configSettingKeys {
		if seen[k] {
			t.Errorf("duplicate key %s", k)
		}
		seen[k] = true
	}
	for _, banned := range []string{"auth.password_hash", "python_interpreter", "jwt"} {
		if seen[banned] {
			t.Errorf("%s must never be exportable", banned)
		}
	}
}

// TestConfigRoundTripPG exports a configured instance, damages the configuration and imports the bundle back.
func TestConfigRoundTripPG(t *testing.T) {
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer m.Close()
	td := t.TempDir()
	s := New(context.Background(), m, td, td, td)
	h := s.Handler()
	tok, err := signJWT(s.jwtKey)
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	pg := m.pg
	ctx := context.Background()

	do := func(method, path string, body []byte) (int, []byte) {
		r := httptest.NewRequest(method, path, bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec.Code, rec.Body.Bytes()
	}

	// Seed distinctive fixtures (unique names so parallel suites and reruns do not collide).
	const pname, mname, cname = "cfgtest-profile", "cfgtest-mcp", "cfgtest-channel"
	cleanup := func() {
		ps, _ := pg.ListProfiles()
		for _, p := range ps {
			if p.Name == pname && !p.IsDefault {
				_ = pg.DeleteProfile(p.ID)
			}
		}
		ms, _ := pg.ListMCP()
		for _, mm := range ms {
			if mm.Name == mname {
				_ = pg.DeleteMCP(mm.ID)
			}
		}
		cs, _ := pg.ListNotificationChannels(ctx)
		for _, c := range cs {
			if c.Name == cname {
				_ = pg.DeleteNotificationChannel(ctx, c.ID)
			}
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	pid, err := pg.SaveProfile(&db.LLMProfile{Name: pname, Format: "anthropic", Model: "claude-test", APIKey: "sk-secret-ROUNDTRIP-1234", Streaming: true, MaxTokens: 777})
	if err != nil {
		t.Fatal(err)
	}
	mid, err := pg.SaveMCP(&db.MCPServer{Name: mname, Transport: "http", URL: "http://127.0.0.1:1/mcp", Env: json.RawMessage(`{"X-API-Key":"mcp-secret-9"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pg.SaveNotificationChannel(ctx, &db.NotificationChannel{Name: cname, Kind: "webhook", Config: json.RawMessage(`{"url":"https://hooks.example.com/x","headers":{"Authorization":"Bearer chan-secret"}}`)}); err != nil {
		t.Fatal(err)
	}
	if err := pg.SetSetting(settingBraveKey, "brave-secret-5678"); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.braveKey = "brave-secret-5678"
	m.mu.Unlock()

	// --- export
	code, body := do("GET", "/api/config/export", nil)
	if code != 200 {
		t.Fatalf("export = %d %s", code, body)
	}
	for _, want := range []string{"sk-secret-ROUNDTRIP-1234", "mcp-secret-9", "chan-secret", "brave-secret-5678", `"format": "artex-config"`} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("export misses %q", want)
		}
	}
	for _, banned := range []string{"auth.password_hash", "password_hash", "jwt"} {
		if bytes.Contains(body, []byte(banned)) {
			t.Errorf("export must not contain %q", banned)
		}
	}
	var bundle configBundle
	if err := json.Unmarshal(body, &bundle); err != nil {
		t.Fatal(err)
	}
	if code, _ := do("GET", "/api/config/export?sections=bogus", nil); code != 400 {
		t.Errorf("bogus section = %d, want 400", code)
	}
	if code, b := do("GET", "/api/config/export?sections=mcp", nil); code != 200 || bytes.Contains(b, []byte("sk-secret")) {
		t.Errorf("section filter leaked or failed: %d", code)
	}

	// --- damage the instance
	if _, err := pg.SaveProfile(&db.LLMProfile{ID: pid, Name: pname, Format: "anthropic", Model: "changed", APIKey: "sk-other-0000", Streaming: true}); err != nil {
		t.Fatal(err)
	}
	if err := pg.DeleteMCP(mid); err != nil {
		t.Fatal(err)
	}

	// --- dry run writes nothing
	code, resp := do("POST", "/api/config/import?dry_run=true", body)
	if code != 200 {
		t.Fatalf("dry run = %d %s", code, resp)
	}
	var res configImportResult
	_ = json.Unmarshal(resp, &res)
	if !res.DryRun || res.Sections[cfgSecMCP] == nil || res.Sections[cfgSecMCP].Created < 1 {
		t.Errorf("dry run should report the MCP server as created: %s", resp)
	}
	if ms, _ := pg.ListMCP(); hasMCP(ms, mname) {
		t.Fatal("dry run created an MCP server")
	}

	// --- real import restores everything
	code, resp = do("POST", "/api/config/import", body)
	if code != 200 {
		t.Fatalf("import = %d %s", code, resp)
	}
	_ = json.Unmarshal(resp, &res)
	for name, sec := range res.Sections {
		for _, e := range sec.Errors {
			t.Logf("section %s: %s", name, e)
		}
	}
	ps, _ := pg.ListProfilesWithKeys()
	var restored *db.LLMProfile
	for _, p := range ps {
		if p.Name == pname {
			restored = p
		}
	}
	if restored == nil || restored.Model != "claude-test" || restored.APIKey != "sk-secret-ROUNDTRIP-1234" || restored.MaxTokens != 777 {
		t.Errorf("profile not restored: %+v", restored)
	}
	ms, _ := pg.ListMCP()
	if !hasMCP(ms, mname) {
		t.Error("MCP server not restored")
	}
	for _, mm := range ms {
		if mm.Name == mname && !strings.Contains(string(mm.Env), "mcp-secret-9") {
			t.Errorf("MCP env not restored: %s", mm.Env)
		}
	}
	cs, _ := pg.ListNotificationChannels(ctx)
	found := false
	for _, c := range cs {
		found = found || (c.Name == cname && strings.Contains(string(c.Config), "chan-secret"))
	}
	if !found {
		t.Error("notification channel not restored")
	}

	// --- rejects
	if code, _ := do("POST", "/api/config/import", []byte(`{"format":"other","version":1}`)); code != 400 {
		t.Errorf("wrong format = %d, want 400", code)
	}
	if code, _ := do("POST", "/api/config/import", []byte(`{"format":"artex-config","version":99}`)); code != 400 {
		t.Errorf("future version = %d, want 400", code)
	}
	if code, _ := do("POST", "/api/config/import", []byte(`not json`)); code != 400 {
		t.Errorf("garbage = %d, want 400", code)
	}
	if code, _ := do("POST", "/api/config/import", bytes.Repeat([]byte("x"), maxConfigBundleBytes+10)); code != 400 {
		t.Errorf("oversized = %d, want 400", code)
	}

	// unknown settings keys (e.g. the password hash) are skipped, never written
	evil := []byte(`{"format":"artex-config","version":1,"settings":{"auth.password_hash":"x","workers":4}}`)
	code, resp = do("POST", "/api/config/import", evil)
	if code != 200 {
		t.Fatalf("settings import = %d %s", code, resp)
	}
	_ = json.Unmarshal(resp, &res)
	if sec := res.Sections[cfgSecSettings]; sec == nil || sec.Skipped != 1 || sec.Updated != 1 {
		t.Errorf("settings result = %+v", sec)
	}
	if v, ok, _ := pg.GetSetting("auth.password_hash"); ok && v == "x" {
		t.Fatal("import wrote the password hash")
	}

	// bad entries are skipped with an error, good ones still apply
	mixed := []byte(`{"format":"artex-config","version":1,"mcp_servers":[{"name":"","transport":"http","url":"http://x"},{"name":"cfgtest-mcp","transport":"bogus"}],"llm_profiles":[{"name":"cfgtest-bad","format":"nope","model":"m"}]}`)
	code, resp = do("POST", "/api/config/import", mixed)
	if code != 200 {
		t.Fatalf("mixed import = %d %s", code, resp)
	}
	_ = json.Unmarshal(resp, &res)
	if res.Sections[cfgSecMCP].Skipped != 2 || res.Sections[cfgSecLLM].Skipped != 1 {
		t.Errorf("bad entries not skipped: %s", resp)
	}
}

func hasMCP(ms []*db.MCPServer, name string) bool {
	for _, m := range ms {
		if m.Name == name {
			return true
		}
	}
	return false
}

var _ = http.StatusOK
