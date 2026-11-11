package db

import "testing"

func TestClassificationNormalize(t *testing.T) {
	c := &FindingClassification{CVSSVector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H", CWE: []string{"79", "CWE-79"}, Attack: []string{"t1190"}, OWASP: []string{"a03:2021"}}
	if err := c.Normalize(); err != nil {
		t.Fatal(err)
	}
	if c.CVSSScore == nil || *c.CVSSScore != 9.8 || c.CVSSSeverity != "critical" || len(c.CWE) != 1 || c.CWE[0] != "CWE-79" || c.Attack[0] != "T1190" || c.OWASP[0] != "A03:2021" {
		t.Fatalf("%+v", c)
	}
	// a client-supplied score for a 3.x vector is ignored
	bogus := 1.0
	c2 := &FindingClassification{CVSSVector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H", CVSSScore: &bogus}
	if err := c2.Normalize(); err != nil || *c2.CVSSScore != 9.8 {
		t.Fatalf("score must be computed: %v %+v", err, c2)
	}
	// 4.0 needs a manual score
	v4 := &FindingClassification{CVSSVector: "CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:H/VI:H/VA:H/SC:N/SI:N/SA:N"}
	if err := v4.Normalize(); err == nil {
		t.Error("a 4.0 vector without a score must be rejected")
	}
	score := 9.3
	v4.CVSSScore = &score
	if err := v4.Normalize(); err != nil || v4.CVSSSeverity != "critical" {
		t.Errorf("4.0 with score: %v %+v", err, v4)
	}
	for _, bad := range []*FindingClassification{
		{CWE: []string{"nope"}}, {OWASP: []string{"A99:2021"}}, {Attack: []string{"X1"}}, {CVSSVector: "junk"},
	} {
		if err := bad.Normalize(); err == nil {
			t.Errorf("%+v should be rejected", bad)
		}
	}
}

func TestClassificationPG(t *testing.T) {
	d := openTargetDB(t)
	task := targetTask(t, d, TaskCreateOptions{})
	fid, err := d.AddFinding(task.ID, 0, "sqli", "SQL injection in login", "high", "summary", "evidence", "w", nil)
	if err != nil || fid == 0 {
		t.Skipf("cannot create a finding here: %v", err)
	}
	empty, err := d.GetFindingClassification(fid)
	if err != nil || empty.CVSSScore != nil || len(empty.CWE) != 0 {
		t.Fatalf("empty: %v %+v", err, empty)
	}
	if err := d.SaveFindingClassification(&FindingClassification{FindingID: fid, CVSSVector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H", CWE: []string{"89"}, Remediation: " Use parameterised queries. "}); err != nil {
		t.Fatal(err)
	}
	got, _ := d.GetFindingClassification(fid)
	if got.CVSSScore == nil || *got.CVSSScore != 9.8 || got.CWE[0] != "CWE-89" || got.Remediation != "Use parameterised queries." {
		t.Fatalf("%+v", got)
	}
	m, err := d.ListFindingClassifications([]int64{fid, 999999999})
	if err != nil || len(m) != 1 {
		t.Fatalf("list: %v %v", err, m)
	}
	if err := d.SaveFindingClassification(&FindingClassification{FindingID: 999999999}); err == nil {
		t.Error("an unknown finding must be rejected")
	}
}
