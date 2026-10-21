package main

import (
	"context"
	"log"
	"time"

	"github.com/PowellHabwe/Savannah-HMIS/libs/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

type window struct {
	Weekdays []int
	Start    string
	End      string
}

type doctorSeed struct {
	ID        string
	FullName  string
	Specialty string
	Windows   []window
}

type patientSeed struct {
	ID       string
	FullName string
	Phone    string
	Email    string
}

var doctors = []doctorSeed{
	{"d0000000-0000-4000-8000-000000000001", "Dr. Amina Wanjiku", "General Practice",
		[]window{{[]int{1, 2, 3, 4, 5}, "09:00", "17:00"}}},
	{"d0000000-0000-4000-8000-000000000002", "Dr. Brian Otieno", "Paediatrics",
		[]window{{[]int{1, 3, 5}, "08:00", "14:00"}}},
	{"d0000000-0000-4000-8000-000000000003", "Dr. Cynthia Mutiso", "Dermatology",
		[]window{{[]int{2, 3, 4, 5, 6}, "10:00", "18:00"}}},
	{"d0000000-0000-4000-8000-000000000004", "Dr. David Kimani", "Cardiology",
		[]window{{[]int{1, 2, 3, 4}, "08:30", "16:30"}}},
	{"d0000000-0000-4000-8000-000000000005", "Dr. Esther Achieng", "Gynaecology",
		[]window{{[]int{1, 2, 4, 5}, "09:00", "15:00"}, {[]int{6}, "09:00", "12:00"}}},
}

var patients = []patientSeed{
	{"a0000000-0000-4000-8000-000000000001", "Grace Njeri", "+254700000001", "grace@example.com"},
	{"a0000000-0000-4000-8000-000000000002", "Peter Mwangi", "+254700000002", "peter@example.com"},
	{"a0000000-0000-4000-8000-000000000003", "Lilian Chebet", "+254700000003", ""},
}

func main() {
	c := config.New()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, c.DB.DSN())
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)

	for _, d := range doctors {
		if _, err := tx.Exec(ctx,
			`INSERT INTO doctor (id, full_name, specialty) VALUES ($1, $2, $3)
			 ON CONFLICT (id) DO UPDATE SET full_name = EXCLUDED.full_name, specialty = EXCLUDED.specialty`,
			d.ID, d.FullName, d.Specialty); err != nil {
			log.Fatalf("seed doctor %s: %v", d.FullName, err)
		}
		for _, w := range d.Windows {
			for _, wd := range w.Weekdays {
				if _, err := tx.Exec(ctx,
					`INSERT INTO doctor_working_hours (doctor_id, weekday, start_time, end_time)
					 VALUES ($1, $2, $3::text::time, $4::text::time)
					 ON CONFLICT (doctor_id, weekday)
					 DO UPDATE SET start_time = EXCLUDED.start_time, end_time = EXCLUDED.end_time`,
					d.ID, wd, w.Start, w.End); err != nil {
					log.Fatalf("seed hours for %s: %v", d.FullName, err)
				}
			}
		}
	}

	for _, p := range patients {
		var email any
		if p.Email != "" {
			email = p.Email
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO patient (id, full_name, phone, email) VALUES ($1, $2, $3, $4)
			 ON CONFLICT (id) DO UPDATE SET full_name = EXCLUDED.full_name, phone = EXCLUDED.phone, email = EXCLUDED.email`,
			p.ID, p.FullName, p.Phone, email); err != nil {
			log.Fatalf("seed patient %s: %v", p.FullName, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		log.Fatalf("commit: %v", err)
	}
	log.Printf("seeded %d doctors and %d patients", len(doctors), len(patients))
}
