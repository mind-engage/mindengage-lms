package agshttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/mind-engage/mindengage-lms/pkg/lti-ags-gradebook/gradebook"
	"golang.org/x/oauth2/clientcredentials"
)

type Client struct {
	http *http.Client
}

type Config struct {
	TokenURL     string
	ClientID     string
	ClientSecret string
	// Optional: Scopes []string
	Timeout time.Duration
}

func New(cfg Config) *Client {
	cc := clientcredentials.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		TokenURL:     cfg.TokenURL,
	}
	h := cc.Client(context.Background())
	if cfg.Timeout > 0 {
		h.Timeout = cfg.Timeout
	}
	return &Client{http: h}
}

func (c *Client) ListLineItems(lineItemsURL string, q map[string]string) ([]gradebook.LineItem, error) {
	u, _ := url.Parse(lineItemsURL)
	p := u.Query()
	for k, v := range q {
		p.Set(k, v)
	}
	u.RawQuery = p.Encode()
	req, _ := http.NewRequest("GET", u.String(), nil)
	req.Header.Set("Accept", "application/vnd.ims.lis.v2.lineitem+json")
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return nil, fmt.Errorf("list line items: %s", res.Status)
	}
	var items []struct {
		ID             string  `json:"id"`
		Label          string  `json:"label"`
		ScoreMaximum   float64 `json:"scoreMaximum"`
		ResourceID     string  `json:"resourceId"`
		ResourceLinkID string  `json:"resourceLinkId"`
	}
	if err := json.NewDecoder(res.Body).Decode(&items); err != nil {
		return nil, err
	}
	out := make([]gradebook.LineItem, 0, len(items))
	for _, it := range items {
		out = append(out, gradebook.LineItem{
			ID: it.ID, Label: it.Label, ScoreMaximum: it.ScoreMaximum,
			ResourceID: it.ResourceID, ResourceLinkID: it.ResourceLinkID,
		})
	}
	return out, nil
}

func (c *Client) CreateLineItem(lineItemsURL string, req gradebook.CreateLineItemReq) (gradebook.LineItem, error) {
	body, _ := json.Marshal(map[string]any{
		"label": req.Label, "scoreMaximum": req.ScoreMaximum,
		"resourceId": req.ResourceID, "resourceLinkId": req.ResourceLinkID,
	})
	httpReq, _ := http.NewRequest("POST", lineItemsURL, bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/vnd.ims.lis.v2.lineitem+json")
	httpReq.Header.Set("Accept", "application/vnd.ims.lis.v2.lineitem+json")
	res, err := c.http.Do(httpReq)
	if err != nil {
		return gradebook.LineItem{}, err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return gradebook.LineItem{}, fmt.Errorf("create line item: %s", res.Status)
	}
	var it struct {
		ID             string  `json:"id"`
		Label          string  `json:"label"`
		ScoreMaximum   float64 `json:"scoreMaximum"`
		ResourceID     string  `json:"resourceId"`
		ResourceLinkID string  `json:"resourceLinkId"`
	}
	if err := json.NewDecoder(res.Body).Decode(&it); err != nil {
		return gradebook.LineItem{}, err
	}
	return gradebook.LineItem{
		ID: it.ID, Label: it.Label, ScoreMaximum: it.ScoreMaximum,
		ResourceID: it.ResourceID, ResourceLinkID: it.ResourceLinkID,
	}, nil
}

func (c *Client) PostScore(lineItemURL string, s gradebook.Score) error {
	body, _ := json.Marshal(map[string]any{
		"userId": s.UserID, "scoreGiven": s.ScoreGiven, "scoreMaximum": s.ScoreMaximum,
		"activityProgress": s.ActivityProgress, "gradingProgress": s.GradingProgress,
		"timestamp": s.Timestamp.Format(time.RFC3339),
	})
	// POST {lineItemURL}/scores
	if lineItemURL[len(lineItemURL)-1] != '/' {
		lineItemURL += "/"
	}
	httpReq, _ := http.NewRequest("POST", lineItemURL+"scores", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/vnd.ims.lis.v1.score+json")
	res, err := c.http.Do(httpReq)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return fmt.Errorf("post score: %s", res.Status)
	}
	return nil
}
