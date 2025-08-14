package lti

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// CreateLineItemReq mirrors the minimal fields you use when creating a line item.
type CreateLineItemReq struct {
	Label          string
	ScoreMaximum   float64
	ResourceID     string
	ResourceLinkID string
}

// AGSClientImpl adapts the concrete *AGSClient (defined in ags.go)
// to a friendlier interface used elsewhere in your codebase.
type AGSClientImpl struct {
	Client *AGSClient
}

// ListLineItems adapts (lineItemsURL + query map) -> AGSClient.ListLineItems signature.
func (a *AGSClientImpl) ListLineItems(ctx context.Context, lineItemsURL string, q map[string]string) ([]LineItem, error) {
	if a == nil || a.Client == nil {
		return nil, errors.New("ags: client is nil")
	}
	// Temporarily point the client at the requested collection URL.
	prev := a.Client.LineItemsURL
	a.Client.LineItemsURL = lineItemsURL
	defer func() { a.Client.LineItemsURL = prev }()

	resourceID := q["resource_id"]
	resourceLinkID := q["resource_link_id"]
	limit := atoi0(q["limit"])
	page := atoi0(q["page"])

	return a.Client.ListLineItems(ctx, resourceID, resourceLinkID, limit, page)
}

// CreateLineItem adapts your request struct to the LineItem used by AGSClient.
func (a *AGSClientImpl) CreateLineItem(ctx context.Context, lineItemsURL string, req CreateLineItemReq) (LineItem, error) {
	if a == nil || a.Client == nil {
		return LineItem{}, errors.New("ags: client is nil")
	}
	prev := a.Client.LineItemsURL
	a.Client.LineItemsURL = lineItemsURL
	defer func() { a.Client.LineItemsURL = prev }()

	li := LineItem{
		Label:          req.Label,
		ScoreMaximum:   req.ScoreMaximum,
		ResourceID:     req.ResourceID,
		ResourceLinkID: req.ResourceLinkID,
	}
	return a.Client.CreateLineItem(ctx, li)
}

// PostScore ensures timestamp is set and forwards to AGSClient.PostScore.
func (a *AGSClientImpl) PostScore(ctx context.Context, lineItemURL string, s Score) error {
	if a == nil || a.Client == nil {
		return errors.New("ags: client is nil")
	}
	if strings.TrimSpace(s.Timestamp) == "" {
		s.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	return a.Client.PostScore(ctx, lineItemURL, s)
}

// NewAGSClientForPlatform constructs an *AGSClientImpl using client_credentials.
func NewAGSClientForPlatform(_ context.Context, platformTokenURL, clientID, clientSecret string) (*AGSClientImpl, error) {
	cl := &AGSClient{
		HTTP:         &http.Client{Timeout: 15 * time.Second},
		TokenURL:     platformTokenURL,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		// Broad default set; List/Create/Post will ask for the specific one they need.
		Scopes: []string{
			"https://purl.imsglobal.org/spec/lti-ags/scope/lineitem",
			"https://purl.imsglobal.org/spec/lti-ags/scope/lineitem.readonly",
			"https://purl.imsglobal.org/spec/lti-ags/scope/score",
			"https://purl.imsglobal.org/spec/lti-ags/scope/result.readonly",
		},
	}
	return &AGSClientImpl{Client: cl}, nil
}

func atoi0(s string) int {
	if s == "" {
		return 0
	}
	n, _ := strconv.Atoi(s)
	return n
}
