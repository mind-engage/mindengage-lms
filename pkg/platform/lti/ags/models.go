package ags

import "time"

type LineItem struct {
	ID             string
	ContextID      string
	ResourceLinkID string
	ResourceID     string
	Label          string
	ScoreMaximum   float64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Score struct {
	UserID           string
	ScoreGiven       *float64
	ScoreMaximum     *float64
	ActivityProgress string
	GradingProgress  string
	Timestamp        time.Time
	Comment          string
}
