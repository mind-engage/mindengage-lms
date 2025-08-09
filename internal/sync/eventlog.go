package syncx

import (
	"context"
	"database/sql"
	"time"
)

type Event struct {
	Offset    int64
	SiteID    string
	Type      string
	Key       string
	DataJSON  string
	CreatedAt int64
}

type EventRepo struct{ db *sql.DB }

func NewEventRepo(db *sql.DB) *EventRepo { return &EventRepo{db: db} }

func (r *EventRepo) Append(ctx context.Context, e Event) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO event_log (site_id, typ, key, data, created_at)
		 VALUES ($1,$2,$3,$4,$5)`,
		e.SiteID, e.Type, e.Key, e.DataJSON, time.Now().Unix())
	return err
}
