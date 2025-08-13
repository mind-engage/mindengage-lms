package gradebook

import (
	"context"
	"fmt"
	"net/url"
	"time"

	// TODO(repo): replace with your lti/ags client import
	ltiags "github.com/mind-engage/mindengage-lms/internal/lti/ags"
)

type AGSClientImpl struct {
	Client *ltiags.Client
}

func (a *AGSClientImpl) ListLineItems(ctx context.Context, lineItemsURL string, q map[string]string) ([]LineItem, error) {
	u, err := url.Parse(lineItemsURL)
	if err != nil {
		return nil, err
	}
	params := u.Query()
	for k, v := range q {
		params.Set(k, v)
	}
	u.RawQuery = params.Encode()

	items, err := a.Client.ListLineItems(ctx, u.String())
	if err != nil {
		return nil, err
	}

	out := make([]LineItem, 0, len(items))
	for _, it := range items {
		out = append(out, LineItem{
			ID:             it.ID, // absolute
			Label:          it.Label,
			ScoreMaximum:   it.ScoreMaximum,
			ResourceID:     it.ResourceID,
			ResourceLinkID: it.ResourceLinkID,
		})
	}
	return out, nil
}

func (a *AGSClientImpl) CreateLineItem(ctx context.Context, lineItemsURL string, req CreateLineItemReq) (LineItem, error) {
	created, err := a.Client.CreateLineItem(ctx, lineItemsURL, ltiags.CreateLineItemRequest{
		Label:          req.Label,
		ScoreMaximum:   req.ScoreMaximum,
		ResourceID:     req.ResourceID,
		ResourceLinkID: req.ResourceLinkID,
	})
	if err != nil {
		return LineItem{}, err
	}
	return LineItem{
		ID:             created.ID,
		Label:          created.Label,
		ScoreMaximum:   created.ScoreMaximum,
		ResourceID:     created.ResourceID,
		ResourceLinkID: created.ResourceLinkID,
	}, nil
}

func (a *AGSClientImpl) PostScore(ctx context.Context, lineItemURL string, s Score) error {
	return a.Client.PostScore(ctx, lineItemURL, ltiags.Score{
		UserID:           s.UserID,
		ScoreGiven:       s.ScoreGiven,
		ScoreMaximum:     s.ScoreMaximum,
		ActivityProgress: s.ActivityProgress,
		GradingProgress:  s.GradingProgress,
		Timestamp:        s.Timestamp.Format(time.RFC3339),
	})
}

// Helper to construct an AGS client for a platform using client credentials.
func NewAGSClientForPlatform(ctx context.Context, platformTokenURL, clientID, clientSecret string) (*AGSClientImpl, error) {
	cl, err := ltiags.NewClient(ltiags.Config{
		TokenURL:     platformTokenURL,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		// TODO(repo): set timeouts, http client, scopes behavior if your impl requires
	})
	if err != nil {
		return nil, fmt.Errorf("ags client: %w", err)
	}
	return &AGSClientImpl{Client: cl}, nil
}
