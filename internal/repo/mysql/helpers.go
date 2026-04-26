package mysql

import (
	"database/sql"
	"time"
)

func timeOrNull(t time.Time) sql.NullTime {
	if t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t, Valid: true}
}

