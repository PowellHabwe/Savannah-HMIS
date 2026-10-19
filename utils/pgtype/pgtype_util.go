package pgtypeUtil

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func StringToPgTypeText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func TimeToPgTimestampTz(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func PgUUIDToGoogleUUID(u pgtype.UUID) (uuid.UUID, error) {
	if !u.Valid {
		return uuid.UUID{}, errors.New("invalid UUID")
	}
	return uuid.UUID(u.Bytes), nil
}

func GoogleUUIDToPgUUID(u uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: u, Valid: true}
}
