package traffic

import "testing"

func TestJournalModeForPersistentNetworkVolume(t *testing.T) {
	for _, mode := range []string{"WAL", "DELETE"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("ARTEX_TRAFFIC_JOURNAL_MODE", mode)
			store, err := Open(t.TempDir(), ":0")
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			var actual string
			if err = store.db.QueryRow(`PRAGMA journal_mode`).Scan(&actual); err != nil {
				t.Fatal(err)
			}
			if actual != "wal" && mode == "WAL" || actual != "delete" && mode == "DELETE" {
				t.Fatalf("journal mode=%s want %s", actual, mode)
			}
		})
	}
	t.Setenv("ARTEX_TRAFFIC_JOURNAL_MODE", "WAL); DROP TABLE exchanges;")
	if _, err := Open(t.TempDir(), ":0"); err == nil {
		t.Fatal("invalid journal mode accepted")
	}
}
