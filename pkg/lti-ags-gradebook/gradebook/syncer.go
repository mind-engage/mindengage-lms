// pkg/gradebook/syncer.go
package gradebook

import (
	"errors"
	"fmt"
	"time"
)

type Clock func() time.Time

type Syncer struct {
	Store Store
	AGS   AGSClient
	Now   Clock
}

func New(store Store, ags AGSClient, now Clock) *Syncer {
	if now == nil {
		now = time.Now
	}
	return &Syncer{Store: store, AGS: ags, Now: now}
}

func (s *Syncer) EnsureLineItem(at Attempt) (GradebookLineItem, error) {
	if li, err := s.Store.FindLineItem(at.ExamID, at.PlatformIssuer, at.DeploymentID, at.ContextID, at.ResourceLinkID); err == nil && li.LineItemURL != "" {
		return li, nil
	}
	link, err := s.Store.GetLatestLinkForContext(at.PlatformIssuer, at.DeploymentID, at.ContextID, at.ResourceLinkID)
	if err != nil {
		return GradebookLineItem{}, fmt.Errorf("no LTI link context: %w", err)
	}
	if link.LineItemsURL == "" {
		return GradebookLineItem{}, errors.New("missing lineitems_url")
	}

	ex, err := s.Store.GetExam(at.ExamID)
	if err != nil {
		return GradebookLineItem{}, fmt.Errorf("exam: %w", err)
	}

	items, err := s.AGS.ListLineItems(link.LineItemsURL, map[string]string{
		"resource_id":      ex.ID,
		"resource_link_id": at.ResourceLinkID,
	})
	if err == nil {
		for _, it := range items {
			if it.ResourceID == ex.ID && it.ResourceLinkID == at.ResourceLinkID {
				return s.Store.UpsertLineItem(GradebookLineItem{
					ExamID: ex.ID, PlatformIssuer: at.PlatformIssuer, DeploymentID: at.DeploymentID,
					ContextID: at.ContextID, ResourceLinkID: at.ResourceLinkID,
					Label: it.Label, ScoreMax: it.ScoreMaximum, LineItemURL: it.ID,
				})
			}
		}
	}
	created, err := s.AGS.CreateLineItem(link.LineItemsURL, CreateLineItemReq{
		Label: ex.Title, ScoreMaximum: ex.MaxPts, ResourceID: ex.ID, ResourceLinkID: at.ResourceLinkID,
	})
	if err != nil {
		return GradebookLineItem{}, fmt.Errorf("create line item: %w", err)
	}
	return s.Store.UpsertLineItem(GradebookLineItem{
		ExamID: ex.ID, PlatformIssuer: at.PlatformIssuer, DeploymentID: at.DeploymentID,
		ContextID: at.ContextID, ResourceLinkID: at.ResourceLinkID,
		Label: created.Label, ScoreMax: created.ScoreMaximum, LineItemURL: created.ID,
	})
}

func (s *Syncer) SyncAttempt(attemptID string) error {
	at, err := s.Store.GetAttempt(attemptID)
	if err != nil {
		return err
	}
	if at.SubmittedAt == nil {
		return errors.New("attempt not submitted")
	}
	_ = s.Store.MarkSyncPending(at.ID)

	li, err := s.EnsureLineItem(at)
	if err != nil {
		_ = s.Store.MarkSyncFailed(at.ID, err.Error())
		return err
	}

	platformUserID, err := s.Store.GetPlatformUserID(at.PlatformIssuer, at.UserID)
	if err != nil || platformUserID == "" {
		_ = s.Store.MarkSyncFailed(at.ID, "no platform user mapping")
		return fmt.Errorf("no platform user mapping for %s", at.UserID)
	}

	ex, err := s.Store.GetExam(at.ExamID)
	if err != nil {
		_ = s.Store.MarkSyncFailed(at.ID, err.Error())
		return err
	}

	if err := s.AGS.PostScore(li.LineItemURL, Score{
		UserID: platformUserID, ScoreGiven: at.Score, ScoreMaximum: ex.MaxPts,
		ActivityProgress: "Completed", GradingProgress: "FullyGraded",
		Timestamp: s.Now(),
	}); err != nil {
		_ = s.Store.MarkSyncFailed(at.ID, err.Error())
		return err
	}
	return s.Store.MarkSyncOK(at.ID)
}
