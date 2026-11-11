package engagement

import (
	"testing"
	"time"
)

func at(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestAlwaysOpenByDefault(t *testing.T) {
	if st := (Window{}).At(at(t, "2026-05-01T12:00:00Z")); !st.Open || !st.NextChange.IsZero() {
		t.Fatalf("%+v", st)
	}
}

func TestPeriodBounds(t *testing.T) {
	s, e := at(t, "2026-05-01T00:00:00Z"), at(t, "2026-05-10T00:00:00Z")
	w := Window{Start: &s, End: &e}
	if st := w.At(at(t, "2026-04-30T23:59:00Z")); st.Open || st.Reason == "" {
		t.Fatalf("before start: %+v", st)
	}
	if st := w.At(at(t, "2026-05-05T10:00:00Z")); !st.Open || !st.NextChange.Equal(e) {
		t.Fatalf("inside: %+v", st)
	}
	if st := w.At(e); st.Open {
		t.Fatalf("end is exclusive: %+v", st)
	}
}

func TestOvernightSlot(t *testing.T) {
	w := Window{TZ: "UTC", Slots: []Slot{{Days: []int{1, 2, 3, 4, 5}, Start: "22:00", End: "06:00"}}}
	// 2026-05-04 is a Monday.
	for _, tc := range []struct {
		when string
		open bool
	}{
		{"2026-05-04T21:59:00Z", false},
		{"2026-05-04T22:00:00Z", true},
		{"2026-05-05T05:59:00Z", true}, // Tuesday morning belongs to Monday's slot
		{"2026-05-05T06:00:00Z", false},
		{"2026-05-09T23:00:00Z", false}, // Saturday night: no Saturday slot
		{"2026-05-10T02:00:00Z", false}, // Sunday early morning would belong to Saturday's slot
		{"2026-05-09T02:00:00Z", true},  // Saturday early morning belongs to Friday's slot
	} {
		if st := w.At(at(t, tc.when)); st.Open != tc.open {
			t.Errorf("%s: open=%v want %v (%+v)", tc.when, st.Open, tc.open, st)
		}
	}
	st := w.At(at(t, "2026-05-04T23:00:00Z"))
	if !st.NextChange.Equal(at(t, "2026-05-05T06:00:00Z")) {
		t.Errorf("next change = %v", st.NextChange)
	}
	closed := w.At(at(t, "2026-05-04T12:00:00Z"))
	if !closed.NextChange.Equal(at(t, "2026-05-04T22:00:00Z")) {
		t.Errorf("next open = %v", closed.NextChange)
	}
}

func TestTimeZoneAndDST(t *testing.T) {
	w := Window{TZ: "Europe/Berlin", Slots: []Slot{{Start: "09:00", End: "17:00"}}}
	// Summer (CEST, UTC+2): 09:00 local = 07:00Z.
	if st := w.At(at(t, "2026-07-01T06:59:00Z")); st.Open {
		t.Error("summer: should be closed before 07:00Z")
	}
	if st := w.At(at(t, "2026-07-01T07:00:00Z")); !st.Open {
		t.Error("summer: should be open at 07:00Z")
	}
	// Winter (CET, UTC+1): 09:00 local = 08:00Z.
	if st := w.At(at(t, "2026-01-15T07:59:00Z")); st.Open {
		t.Error("winter: should be closed before 08:00Z")
	}
	if st := w.At(at(t, "2026-01-15T08:00:00Z")); !st.Open {
		t.Error("winter: should be open at 08:00Z")
	}
}

func TestSlotCappedByPeriodEnd(t *testing.T) {
	e := at(t, "2026-05-04T23:00:00Z")
	w := Window{End: &e, Slots: []Slot{{Start: "22:00", End: "06:00"}}}
	st := w.At(at(t, "2026-05-04T22:30:00Z"))
	if !st.Open || !st.NextChange.Equal(e) {
		t.Fatalf("%+v", st)
	}
	if st := w.At(at(t, "2026-05-04T23:30:00Z")); st.Open {
		t.Fatalf("after end: %+v", st)
	}
}

func TestInvalidIsClosed(t *testing.T) {
	for _, w := range []Window{
		{TZ: "Mars/Base"},
		{Slots: []Slot{{Start: "25:00", End: "01:00"}}},
		{Slots: []Slot{{Start: "09:00", End: "09:00"}}},
		{Slots: []Slot{{Days: []int{8}, Start: "09:00", End: "10:00"}}},
	} {
		if err := w.Validate(); err == nil {
			t.Errorf("%+v should be invalid", w)
		}
		if st := w.At(time.Now()); st.Open {
			t.Errorf("%+v: an invalid window must be closed", w)
		}
	}
}
