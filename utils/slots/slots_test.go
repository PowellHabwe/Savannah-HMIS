package slots

import (
	"errors"
	"testing"
	"time"
)

var eat = time.FixedZone("EAT", 3*60*60)

func at(h, m int) time.Time { return time.Date(2026, 10, 19, h, m, 0, 0, eat) }

var workday = WorkingWindow{Start: 9 * time.Hour, End: 17 * time.Hour}

func TestGenerate(t *testing.T) {
	tests := []struct {
		name      string
		w         WorkingWindow
		wantCount int
		wantFirst time.Time
		wantLast  time.Time
	}{
		{"two hours", WorkingWindow{9 * time.Hour, 11 * time.Hour}, 4, at(9, 0), at(10, 30)},
		{"half-hour offsets", WorkingWindow{8*time.Hour + 30*time.Minute, 16*time.Hour + 30*time.Minute}, 16, at(8, 30), at(16, 0)},
		{"partial trailing slot dropped", WorkingWindow{9 * time.Hour, 10*time.Hour + 45*time.Minute}, 3, at(9, 0), at(10, 0)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Generate(at(0, 0), tc.w, eat)
			if len(got) != tc.wantCount {
				t.Fatalf("count = %d, want %d", len(got), tc.wantCount)
			}
			if !got[0].Equal(tc.wantFirst) || !got[len(got)-1].Equal(tc.wantLast) {
				t.Fatalf("first/last = %v / %v, want %v / %v", got[0], got[len(got)-1], tc.wantFirst, tc.wantLast)
			}
		})
	}
}

func TestAvailable(t *testing.T) {
	all := Generate(at(0, 0), WorkingWindow{9 * time.Hour, 11 * time.Hour}, eat) 

	tests := []struct {
		name    string
		booked  []time.Time
		now     time.Time
		minLead time.Duration
		want    []time.Time
	}{
		{"nothing booked", nil, at(7, 0), time.Hour, []time.Time{at(9, 0), at(9, 30), at(10, 0), at(10, 30)}},
		{"booked slot removed", []time.Time{at(9, 30)}, at(7, 0), time.Hour, []time.Time{at(9, 0), at(10, 0), at(10, 30)}},
		{"lead time hides near slots", nil, at(9, 30), 30 * time.Minute, []time.Time{at(10, 0), at(10, 30)}},
		{"slot exactly at cut-off is kept", nil, at(9, 0), time.Hour, []time.Time{at(10, 0), at(10, 30)}},
		{"everything in the past", nil, at(12, 0), time.Hour, []time.Time{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Available(all, tc.booked, tc.now, tc.minLead)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if !got[i].Equal(tc.want[i]) {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestValidateBooking(t *testing.T) {
	tests := []struct {
		name    string
		start   time.Time
		w       *WorkingWindow
		now     time.Time
		wantErr error
	}{
		{"valid", at(9, 0), &workday, at(7, 0), nil},
		{"valid last slot of day", at(16, 30), &workday, at(7, 0), nil},
		{"valid given in UTC", at(9, 0).UTC(), &workday, at(7, 0), nil},
		{"exactly at lead-time boundary", at(9, 0), &workday, at(8, 0), nil},
		{"unaligned minutes", at(9, 15), &workday, at(7, 0), ErrNotAligned},
		{"non-zero seconds", time.Date(2026, 10, 19, 9, 0, 30, 0, eat), &workday, at(7, 0), ErrNotAligned},
		{"in the past", at(9, 0), &workday, at(10, 0), ErrInPast},
		{"within one hour of now", at(9, 0), &workday, at(8, 30), ErrTooSoon},
		{"before working hours", at(8, 30), &workday, at(7, 0), ErrOutsideWorkingHours},
		{"starts at closing time", at(17, 0), &workday, at(7, 0), ErrOutsideWorkingHours},
		{"doctor does not work that day", at(9, 0), nil, at(7, 0), ErrOutsideWorkingHours},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateBooking(tc.start, tc.w, eat, tc.now, time.Hour)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestWindowFromMicros(t *testing.T) {
	w := WindowFromMicros(9*3600*1_000_000, 17*3600*1_000_000)
	if w.Start != 9*time.Hour || w.End != 17*time.Hour {
		t.Fatalf("got %+v", w)
	}
}

func TestParseDate(t *testing.T) {
	if _, err := ParseDate("2026-10-19", eat); err != nil {
		t.Fatalf("valid date rejected: %v", err)
	}
	for _, bad := range []string{"19-10-2026", "2026-13-01", "", "tomorrow"} {
		if _, err := ParseDate(bad, eat); err == nil {
			t.Fatalf("%q should be rejected", bad)
		}
	}
}
