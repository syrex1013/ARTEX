// Package engagement holds the pure logic of an engagement that has no database or server dependency,
// so it can be unit-tested on its own: the testing-window calculation.
package engagement

import (
	"fmt"
	"strings"
	"time"
)

// Slot is one recurring daily testing slot. Days are ISO weekdays (1 = Monday .. 7 = Sunday); an empty list
// means every day. Start and End are wall-clock "HH:MM" in the window's time zone. When End is not after
// Start the slot runs past midnight and belongs to the day it starts on.
type Slot struct {
	Days  []int  `json:"days"`
	Start string `json:"start"`
	End   string `json:"end"`
}

// Window is an engagement's testing window. The zero value is "always open".
type Window struct {
	TZ    string     // IANA zone; "" = UTC
	Start *time.Time // the engagement may not run before this instant
	End   *time.Time // ... or at / after this instant
	Slots []Slot     // when non-empty, testing is only open inside one of them
}

// Status is the state of a window at an instant.
type Status struct {
	Open       bool
	Reason     string    // why it is closed ("" when open)
	NextChange time.Time // zero when the state never changes again
}

func parseHM(s string) (int, error) {
	var h, m int
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d:%d", &h, &m); err != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("invalid time %q, want HH:MM", s)
	}
	return h*60 + m, nil
}

// Validate reports a configuration error (bad zone, bad time, bad weekday, empty slot).
func (w Window) Validate() error {
	if _, err := w.location(); err != nil {
		return err
	}
	if w.Start != nil && w.End != nil && !w.End.After(*w.Start) {
		return fmt.Errorf("the window must end after it starts")
	}
	for i, s := range w.Slots {
		a, err := parseHM(s.Start)
		if err != nil {
			return fmt.Errorf("slot %d: %w", i+1, err)
		}
		b, err := parseHM(s.End)
		if err != nil {
			return fmt.Errorf("slot %d: %w", i+1, err)
		}
		if a == b {
			return fmt.Errorf("slot %d: start and end are equal", i+1)
		}
		for _, d := range s.Days {
			if d < 1 || d > 7 {
				return fmt.Errorf("slot %d: weekday %d is not 1..7", i+1, d)
			}
		}
	}
	return nil
}

func (w Window) location() (*time.Location, error) {
	if strings.TrimSpace(w.TZ) == "" {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(strings.TrimSpace(w.TZ))
	if err != nil {
		return nil, fmt.Errorf("unknown time zone %q", w.TZ)
	}
	return loc, nil
}

func isoWeekday(t time.Time) int {
	d := int(t.Weekday())
	if d == 0 {
		return 7
	}
	return d
}

func (s Slot) onDay(d int) bool {
	if len(s.Days) == 0 {
		return true
	}
	for _, x := range s.Days {
		if x == d {
			return true
		}
	}
	return false
}

// slotInterval returns the [start,end) instants of the slot instance that starts on the calendar day of base
// (in loc). Midnight arithmetic goes through time.Date so daylight-saving shifts stay correct.
func slotInterval(s Slot, base time.Time, loc *time.Location) (time.Time, time.Time) {
	a, _ := parseHM(s.Start)
	b, _ := parseHM(s.End)
	y, m, d := base.In(loc).Date()
	start := time.Date(y, m, d, a/60, a%60, 0, 0, loc)
	endDay := d
	if b <= a {
		endDay = d + 1
	}
	end := time.Date(y, m, endDay, b/60, b%60, 0, 0, loc)
	return start, end
}

// At evaluates the window at now. An invalid configuration is treated as closed, with the error as the reason:
// a window nobody can read must not silently allow testing.
func (w Window) At(now time.Time) Status {
	if err := w.Validate(); err != nil {
		return Status{Open: false, Reason: "the testing window is misconfigured: " + err.Error()}
	}
	loc, _ := w.location()
	if w.Start != nil && now.Before(*w.Start) {
		return Status{Open: false, Reason: "the engagement's testing period has not started", NextChange: w.openAfter(*w.Start, loc)}
	}
	if w.End != nil && !now.Before(*w.End) {
		return Status{Open: false, Reason: "the engagement's testing period has ended"}
	}
	if len(w.Slots) == 0 {
		st := Status{Open: true}
		if w.End != nil {
			st.NextChange = *w.End
		}
		return st
	}
	// Open if any slot instance starting yesterday, today or (never) tomorrow covers now.
	var openUntil time.Time
	for _, s := range w.Slots {
		for off := -1; off <= 0; off++ {
			base := now.In(loc).AddDate(0, 0, off)
			if !s.onDay(isoWeekday(base)) {
				continue
			}
			a, b := slotInterval(s, base, loc)
			if !now.Before(a) && now.Before(b) && b.After(openUntil) {
				openUntil = b
			}
		}
	}
	if !openUntil.IsZero() {
		if w.End != nil && w.End.Before(openUntil) {
			openUntil = *w.End
		}
		return Status{Open: true, NextChange: openUntil}
	}
	return Status{Open: false, Reason: "outside the daily testing slots", NextChange: w.openAfter(now, loc)}
}

// openAfter finds the next instant at or after t at which a slot opens (zero when none within 8 days).
func (w Window) openAfter(t time.Time, loc *time.Location) time.Time {
	if len(w.Slots) == 0 {
		return t
	}
	var best time.Time
	for off := 0; off <= 8; off++ {
		base := t.In(loc).AddDate(0, 0, off)
		for _, s := range w.Slots {
			if !s.onDay(isoWeekday(base)) {
				continue
			}
			a, _ := slotInterval(s, base, loc)
			if a.Before(t) {
				continue
			}
			if best.IsZero() || a.Before(best) {
				best = a
			}
		}
		if !best.IsZero() {
			break
		}
	}
	if !best.IsZero() && w.End != nil && !best.Before(*w.End) {
		return time.Time{}
	}
	return best
}
