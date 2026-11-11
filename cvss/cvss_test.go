package cvss

import "testing"

// Reference scores published with the CVSS 3.1 specification and the NVD calculator.
func TestScoreGolden(t *testing.T) {
	for _, tc := range []struct {
		vector string
		score  float64
		sev    string
	}{
		{"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H", 9.8, "critical"},
		{"CVSS:3.1/AV:N/AC:L/PR:N/UI:R/S:C/C:L/I:L/A:N", 6.1, "medium"},
		{"CVSS:3.1/AV:L/AC:L/PR:L/UI:N/S:U/C:H/I:H/A:H", 7.8, "high"},
		{"CVSS:3.1/AV:N/AC:H/PR:N/UI:N/S:U/C:H/I:N/A:N", 5.9, "medium"},
		{"CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:C/C:H/I:H/A:H", 9.9, "critical"},
		{"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:C/C:H/I:H/A:H", 10.0, "critical"},
		{"CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:L/I:N/A:N", 4.3, "medium"},
		{"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N", 7.5, "high"},
		{"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:N", 0.0, "none"},
		{"CVSS:3.0/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H", 9.8, "critical"},
	} {
		r, err := Score(tc.vector)
		if err != nil {
			t.Fatalf("%s: %v", tc.vector, err)
		}
		if r.Score != tc.score || r.Severity != tc.sev {
			t.Errorf("%s = %.1f %s, want %.1f %s", tc.vector, r.Score, r.Severity, tc.score, tc.sev)
		}
	}
}

func TestMetricOrderIsNormalised(t *testing.T) {
	r, err := Score("CVSS:3.1/A:H/I:H/C:H/S:U/UI:N/PR:N/AC:L/AV:N")
	if err != nil || r.Vector != "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H" || r.Score != 9.8 {
		t.Fatalf("%v %+v", err, r)
	}
}

func TestInvalidVectors(t *testing.T) {
	for _, v := range []string{
		"", "AV:N/AC:L", "CVSS:4.0/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H",
		"CVSS:3.1/AV:X/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H",
		"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H",
		"CVSS:3.1/AV:N/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H",
		"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:",
	} {
		if _, err := Score(v); err == nil {
			t.Errorf("%q should be rejected", v)
		}
	}
}

func TestRoundUp(t *testing.T) {
	for in, want := range map[float64]float64{4.0: 4.0, 4.02: 4.1, 4.000001: 4.0, 0.0: 0.0, 9.95: 10.0} {
		if got := roundUp(in); got != want {
			t.Errorf("roundUp(%v) = %v, want %v", in, got, want)
		}
	}
}
