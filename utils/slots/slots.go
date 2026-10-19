package slots

import (
	"errors"
	"fmt"
	"time"
)

const SlotDuration = 30 * time.Minute

var (
	ErrNotAligned          = errors.New("slot start must be on the hour or half hour")
	ErrInPast              = errors.New("cannot book a slot in the past")
	ErrTooSoon             = errors.New("slot is too soon")
	ErrOutsideWorkingHours = errors.New("slot is outside the doctor's working hours")
)

type WorkingWindow struct {
	Start time.Duration
	End   time.Duration
}

func WindowFromMicros(startMicros, endMicros int64) WorkingWindow {
	return WorkingWindow{
		Start: time.Duration(startMicros) * time.Microsecond,
		End:   time.Duration(endMicros) * time.Microsecond,
	}
}

func ParseDate(s string, loc *time.Location) (time.Time, error) {
	d, err := time.ParseInLocation("2006-01-02", s, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("date must be in YYYY-MM-DD format")
	}
	return d, nil
}

func DayBounds(date time.Time, loc *time.Location) (time.Time, time.Time) {
	y, m, d := date.Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, loc)
	return start, time.Date(y, m, d+1, 0, 0, 0, 0, loc)
}

func atOffset(date time.Time, off time.Duration, loc *time.Location) time.Time {
	y, m, d := date.Date()
	mins := int(off / time.Minute)
	return time.Date(y, m, d, mins/60, mins%60, 0, 0, loc)
}

func Generate(date time.Time, w WorkingWindow, loc *time.Location) []time.Time {
	start := atOffset(date, w.Start, loc)
	end := atOffset(date, w.End, loc)

	var out []time.Time
	for t := start; !t.Add(SlotDuration).After(end); t = t.Add(SlotDuration) {
		out = append(out, t)
	}
	return out
}

func Available(all, booked []time.Time, now time.Time, minLead time.Duration) []time.Time {
	taken := make(map[int64]struct{}, len(booked))
	for _, b := range booked {
		taken[b.Unix()] = struct{}{}
	}
	earliest := now.Add(minLead)

	out := make([]time.Time, 0, len(all))
	for _, s := range all {
		if s.Before(earliest) {
			continue
		}
		if _, isTaken := taken[s.Unix()]; isTaken {
			continue
		}
		out = append(out, s)
	}
	return out
}


func ValidateBooking(
	start time.Time,
	w *WorkingWindow,
	loc *time.Location,
	now time.Time,
	minLead time.Duration,
) error {
	local := start.In(loc)

	if local.Minute()%int(SlotDuration/time.Minute) != 0 || local.Second() != 0 || local.Nanosecond() != 0 {
		return ErrNotAligned
	}
	if start.Before(now) {
		return ErrInPast
	}
	if start.Before(now.Add(minLead)) {
		return fmt.Errorf("%w: bookings must be made at least %d minutes in advance",
			ErrTooSoon, int(minLead/time.Minute))
	}

	offset := time.Duration(local.Hour())*time.Hour + time.Duration(local.Minute())*time.Minute
	if w == nil || offset < w.Start || offset+SlotDuration > w.End {
		return ErrOutsideWorkingHours
	}
	return nil
}
