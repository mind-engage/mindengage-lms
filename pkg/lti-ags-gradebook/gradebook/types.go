// pkg/gradebook/types.go
package gradebook

import "time"

type Exam struct {
	ID     string
	Title  string
	MaxPts float64
}

type Attempt struct {
	ID, ExamID, UserID string
	Score              float64
	SubmittedAt        *time.Time
	PlatformIssuer     string
	DeploymentID       string
	ContextID          string
	ResourceLinkID     string
}

type LTILink struct {
	PlatformIssuer, DeploymentID, ContextID, ResourceLinkID string
	LineItemsURL                                            string
	Scopes                                                  []string
}

type GradebookLineItem struct {
	ID                                                              int64
	ExamID, PlatformIssuer, DeploymentID, ContextID, ResourceLinkID string
	Label                                                           string
	ScoreMax                                                        float64
	LineItemURL                                                     string // absolute URL
}

// Store: implement this in your app, or use pkg/sqlstore.Store
type Store interface {
	GetExam(id string) (Exam, error)
	GetAttempt(id string) (Attempt, error)

	GetLatestLinkForContext(issuer, dep, ctx, rl string) (LTILink, error)
	UpsertLineItem(li GradebookLineItem) (GradebookLineItem, error)
	FindLineItem(examID, issuer, dep, ctx, rl string) (GradebookLineItem, error)
	GetPlatformUserID(issuer, localUserID string) (string, error)

	MarkSyncPending(attemptID string) error
	MarkSyncOK(attemptID string) error
	MarkSyncFailed(attemptID, lastErr string) error

	// Optional, used by helpers (agshttp): fetch platform client creds
	GetPlatform(issuer string) (Platform, error)
}

type Platform struct {
	Issuer, ClientID, TokenURL, JWKSURL, AuthURL string
}

type LineItem struct {
	ID, Label, ResourceID, ResourceLinkID string
	ScoreMaximum                          float64
}

type CreateLineItemReq struct {
	Label          string
	ScoreMaximum   float64
	ResourceID     string
	ResourceLinkID string
}

type Score struct {
	UserID, ActivityProgress, GradingProgress string
	ScoreGiven, ScoreMaximum                  float64
	Timestamp                                 time.Time
}

type AGSClient interface {
	ListLineItems(lineItemsURL string, q map[string]string) ([]LineItem, error)
	CreateLineItem(lineItemsURL string, req CreateLineItemReq) (LineItem, error)
	PostScore(lineItemURL string, s Score) error
}
