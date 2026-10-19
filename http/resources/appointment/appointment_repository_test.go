package appointment

// Integration tests: they run against a real Postgres because the guarantee
// we care most about (no double booking) is enforced by the database.
//
// Set TEST_DATABASE_URL to run them; otherwise they are skipped, so a plain
// `go test ./...` still works on a machine without a database.
//
//   $env:TEST_DATABASE_URL="postgres://clinic:clinicpass@localhost:5436/clinic?sslmode=disable"
//
// Each test creates its own doctor and patients with random IDs and deletes
// them afterwards, so it never touches the seed data.

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	db "github.com/PowellHabwe/Savannah-HMIS/db/sqlc"
	httpconfig "github.com/PowellHabwe/Savannah-HMIS/http"
	"github.com/PowellHabwe/Savannah-HMIS/libs/apperr"
	"github.com/PowellHabwe/Savannah-HMIS/libs/config"
	"github.com/PowellHabwe/Savannah-HMIS/utils/slots"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var eat = time.FixedZone("EAT", 3*60*60)


var fixedNow = time.Date(2026, 10, 16, 10, 0, 0, 0, eat)

func slot(h, m int) time.Time { return time.Date(2026, 10, 19, h, m, 0, 0, eat) }

type fixture struct {
	repo     *AppointmentRepository
	doctor   uuid.UUID
	patients []uuid.UUID
}

func newFixture(t *testing.T, patientCount int) *fixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}

	repo := NewAppointmentRepository(&httpconfig.ControllerParams{
		Queries:  db.New(pool),
		Pool:     pool,
		Config:   &config.Config{Clinic: config.ClinicConfig{MinLeadMinutes: 60}},
		Location: eat,
	})
	repo.now = func() time.Time { return fixedNow }

	f := &fixture{repo: repo, doctor: uuid.New()}

	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("setup %q: %v", sql, err)
		}
	}

	mustExec(`INSERT INTO doctor (id, full_name, specialty) VALUES ($1, 'Test Doctor', 'Testing')`, f.doctor)
	mustExec(`INSERT INTO doctor_working_hours (doctor_id, weekday, start_time, end_time)
	          VALUES ($1, 1, TIME '09:00', TIME '17:00')`, f.doctor)
	for i := 0; i < patientCount; i++ {
		id := uuid.New()
		mustExec(`INSERT INTO patient (id, full_name, phone) VALUES ($1, 'Test Patient', '+254700000000')`, id)
		f.patients = append(f.patients, id)
	}

	t.Cleanup(func() {
		bg := context.Background()
		pool.Exec(bg, `DELETE FROM appointment WHERE doctor_id = $1`, f.doctor)
		for _, p := range f.patients {
			pool.Exec(bg, `DELETE FROM patient WHERE id = $1`, p)
		}
		pool.Exec(bg, `DELETE FROM doctor WHERE id = $1`, f.doctor)
	})
	return f
}

func (f *fixture) book(patient int, start time.Time) (*AppointmentResponse, error) {
	return f.repo.Book(context.Background(), &BookParams{
		DoctorID:  f.doctor,
		PatientID: f.patients[patient],
		SlotStart: start,
	})
}

func (f *fixture) mustBook(t *testing.T, patient int, start time.Time) *AppointmentResponse {
	t.Helper()
	a, err := f.book(patient, start)
	if err != nil {
		t.Fatalf("book %v: %v", start, err)
	}
	return a
}

func TestBook_SameSlotTwice_Conflicts(t *testing.T) {
	f := newFixture(t, 2)
	f.mustBook(t, 0, slot(9, 0))

	_, err := f.book(1, slot(9, 0))
	if !errors.Is(err, apperr.ErrSlotTaken) {
		t.Fatalf("err = %v, want ErrSlotTaken", err)
	}
}

func TestBook_ConcurrentSameSlot_OnlyOneWins(t *testing.T) {
	const n = 10
	f := newFixture(t, n)

	start := make(chan struct{})
	results := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(patient int) {
			defer wg.Done()
			<-start 
			_, err := f.book(patient, slot(9, 0))
			results <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)

	var won, taken, other int
	for err := range results {
		switch {
		case err == nil:
			won++
		case errors.Is(err, apperr.ErrSlotTaken):
			taken++
		default:
			other++
			t.Logf("unexpected error: %v", err)
		}
	}
	if won != 1 || taken != n-1 || other != 0 {
		t.Fatalf("won=%d taken=%d other=%d, want 1/%d/0", won, taken, other, n-1)
	}
}

func TestBook_RuleViolations(t *testing.T) {
	f := newFixture(t, 1)

	tests := []struct {
		name    string
		now     time.Time
		start   time.Time
		wantErr error
	}{
		{"outside working hours", fixedNow, slot(7, 0), slots.ErrOutsideWorkingHours},
		{"unaligned", fixedNow, slot(9, 15), slots.ErrNotAligned},
		{"in the past", fixedNow, time.Date(2026, 10, 12, 9, 0, 0, 0, eat), slots.ErrInPast},
		{"within one hour of now", slot(8, 30), slot(9, 0), slots.ErrTooSoon},
		{"doctor off on Sunday", fixedNow, time.Date(2026, 10, 18, 9, 0, 0, 0, eat), slots.ErrOutsideWorkingHours},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			now := tc.now
			f.repo.now = func() time.Time { return now }
			_, err := f.book(0, tc.start)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestBook_UnknownDoctorAndPatient(t *testing.T) {
	f := newFixture(t, 1)
	ctx := context.Background()

	_, err := f.repo.Book(ctx, &BookParams{DoctorID: uuid.New(), PatientID: f.patients[0], SlotStart: slot(9, 0)})
	if !errors.Is(err, apperr.ErrDoctorNotFound) {
		t.Fatalf("err = %v, want ErrDoctorNotFound", err)
	}
	_, err = f.repo.Book(ctx, &BookParams{DoctorID: f.doctor, PatientID: uuid.New(), SlotStart: slot(9, 0)})
	if !errors.Is(err, apperr.ErrPatientNotFound) {
		t.Fatalf("err = %v, want ErrPatientNotFound", err)
	}
}

func TestPatient_CannotBeInTwoPlacesAtOnce(t *testing.T) {
	f := newFixture(t, 1)
	f.mustBook(t, 0, slot(9, 0))

	otherDoctor := uuid.New()
	pool := f.repo.Pool
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO doctor (id, full_name, specialty) VALUES ($1, 'Second', 'Testing')`, otherDoctor); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO doctor_working_hours (doctor_id, weekday, start_time, end_time)
	                             VALUES ($1, 1, TIME '09:00', TIME '17:00')`, otherDoctor); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM appointment WHERE doctor_id = $1`, otherDoctor)
		pool.Exec(context.Background(), `DELETE FROM doctor WHERE id = $1`, otherDoctor)
	})

	_, err := f.repo.Book(ctx, &BookParams{DoctorID: otherDoctor, PatientID: f.patients[0], SlotStart: slot(9, 0)})
	if !errors.Is(err, apperr.ErrPatientBusy) {
		t.Fatalf("err = %v, want ErrPatientBusy", err)
	}
}

func TestCancel_FreesSlotAndCannotRepeat(t *testing.T) {
	f := newFixture(t, 2)
	ctx := context.Background()
	appt := f.mustBook(t, 0, slot(9, 0))

	got, err := f.repo.Cancel(ctx, appt.ID, "travelling")
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if got.Status != "CANCELLED" || got.CancelReason == nil || *got.CancelReason != "travelling" {
		t.Fatalf("unexpected cancelled appointment: %+v", got)
	}

	if _, err := f.repo.Cancel(ctx, appt.ID, "again"); !errors.Is(err, apperr.ErrAlreadyCancelled) {
		t.Fatalf("second cancel err = %v, want ErrAlreadyCancelled", err)
	}

	f.mustBook(t, 1, slot(9, 0))
}

func TestCancel_UnknownAppointment(t *testing.T) {
	f := newFixture(t, 1)
	if _, err := f.repo.Cancel(context.Background(), uuid.New(), "x"); !errors.Is(err, apperr.ErrAppointmentNotFound) {
		t.Fatalf("err = %v, want ErrAppointmentNotFound", err)
	}
}

func TestReschedule_MovesAndFreesOldSlot(t *testing.T) {
	f := newFixture(t, 2)
	ctx := context.Background()
	appt := f.mustBook(t, 0, slot(9, 0))

	moved, err := f.repo.Reschedule(ctx, appt.ID, slot(10, 0))
	if err != nil {
		t.Fatalf("reschedule: %v", err)
	}
	if !moved.SlotStart.Equal(slot(10, 0)) || !moved.SlotEnd.Equal(slot(10, 30)) {
		t.Fatalf("slot = %v - %v", moved.SlotStart, moved.SlotEnd)
	}

	f.mustBook(t, 1, slot(9, 0))
}

func TestReschedule_ToTakenSlot_LeavesOriginalUntouched(t *testing.T) {
	f := newFixture(t, 2)
	ctx := context.Background()
	mine := f.mustBook(t, 0, slot(9, 0))
	f.mustBook(t, 1, slot(10, 0))

	_, err := f.repo.Reschedule(ctx, mine.ID, slot(10, 0))
	if !errors.Is(err, apperr.ErrSlotTaken) {
		t.Fatalf("err = %v, want ErrSlotTaken", err)
	}

	still, err := f.repo.GetByID(ctx, mine.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !still.SlotStart.Equal(slot(9, 0)) || still.Status != "BOOKED" {
		t.Fatalf("original changed: %+v", still)
	}
}

func TestReschedule_ValidatesLikeFreshBooking(t *testing.T) {
	f := newFixture(t, 1)
	appt := f.mustBook(t, 0, slot(9, 0))

	if _, err := f.repo.Reschedule(context.Background(), appt.ID, slot(7, 0)); !errors.Is(err, slots.ErrOutsideWorkingHours) {
		t.Fatalf("err = %v, want ErrOutsideWorkingHours", err)
	}
	if _, err := f.repo.Reschedule(context.Background(), appt.ID, slot(9, 15)); !errors.Is(err, slots.ErrNotAligned) {
		t.Fatalf("err = %v, want ErrNotAligned", err)
	}
}

func TestReschedule_CancelledAppointment_Errors(t *testing.T) {
	f := newFixture(t, 1)
	ctx := context.Background()
	appt := f.mustBook(t, 0, slot(9, 0))
	if _, err := f.repo.Cancel(ctx, appt.ID, "no longer needed"); err != nil {
		t.Fatal(err)
	}

	if _, err := f.repo.Reschedule(ctx, appt.ID, slot(11, 0)); !errors.Is(err, apperr.ErrAlreadyCancelled) {
		t.Fatalf("err = %v, want ErrAlreadyCancelled", err)
	}
}

func TestChange_PastAppointment_Rejected(t *testing.T) {
	f := newFixture(t, 1)
	ctx := context.Background()
	appt := f.mustBook(t, 0, slot(9, 0))

	f.repo.now = func() time.Time { return slot(9, 10) }

	if _, err := f.repo.Cancel(ctx, appt.ID, "late"); !errors.Is(err, apperr.ErrAppointmentPassed) {
		t.Fatalf("cancel err = %v, want ErrAppointmentPassed", err)
	}
	if _, err := f.repo.Reschedule(ctx, appt.ID, slot(11, 0)); !errors.Is(err, apperr.ErrAppointmentPassed) {
		t.Fatalf("reschedule err = %v, want ErrAppointmentPassed", err)
	}
}
