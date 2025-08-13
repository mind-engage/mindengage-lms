// internal/lti/ags.go
package lti

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

/*
MVP AGS client:
- Create/List/Delete Line Items
- Post Scores
- Read Results

Auth: client_credentials (MVP). Later you can swap to JWT client assertion.
*/

// ===== Models (per IMS AGS 2.0 spec, trimmed to what we use) =====

type LineItem struct {
	ID             string   `json:"id,omitempty"`             // Absolute URL for this line item
	ScoreMaximum   float64  `json:"scoreMaximum,omitempty"`   // required when creating
	Label          string   `json:"label,omitempty"`          // teacher-visible label
	ResourceID     string   `json:"resourceId,omitempty"`     // tool-defined grouping (e.g., exam_id)
	ResourceLinkID string   `json:"resourceLinkId,omitempty"` // from launch claim
	Tag            string   `json:"tag,omitempty"`            // tool-defined tag (e.g., attempt_id)
	StartDateTime  string   `json:"startDateTime,omitempty"`  // RFC3339
	EndDateTime    string   `json:"endDateTime,omitempty"`    // RFC3339
	BestScore      *float64 `json:"-"`                        // not standard; keep local if needed
}

type Score struct {
	UserID           string   `json:"userId"`                 // platform's user identifier (sub from NRPS or launch)
	Timestamp        string   `json:"timestamp"`              // RFC3339
	ScoreGiven       *float64 `json:"scoreGiven,omitempty"`   // awarded points
	ScoreMaximum     *float64 `json:"scoreMaximum,omitempty"` // max points (usually equals line item max)
	ActivityProgress string   `json:"activityProgress"`       // Initialized|InProgress|Submitted|Completed
	GradingProgress  string   `json:"gradingProgress"`        // NotReady|Pending|Failed|PendingManual|FullyGraded
	Comment          string   `json:"comment,omitempty"`
}

type Result struct {
	ID            string   `json:"id,omitempty"` // result URL
	UserID        string   `json:"userId,omitempty"`
	ResultScore   *float64 `json:"resultScore,omitempty"` // last known score
	ResultMaximum *float64 `json:"resultMaximum,omitempty"`
	Comment       string   `json:"comment,omitempty"`
	Timestamp     string   `json:"timestamp,omitempty"` // RFC3339
}

// Claim from LTI launch: https://purl.imsglobal.org/spec/lti-ags/claim/endpoint
type endpointClaim struct {
	LineItems string   `json:"lineitems"`
	Scope     []string `json:"scope"`
}

// ===== Client =====

type AGSClient struct {
	HTTP *http.Client

	// OAuth token endpoint + client credentials (MVP).
	TokenURL     string
	ClientID     string
	ClientSecret string

	// From launch claim.
	LineItemsURL string
	Scopes       []string
}

// NewAGSFromLaunch builds a client from launch-time claims + platform creds.
//   - agsLineItemsURL: claim["https://purl.imsglobal.org/spec/lti-ags/claim/endpoint"].lineitems
//   - agsScopes:       claim[".../endpoint"].scope
func NewAGSFromLaunch(tokenURL, clientID, clientSecret, agsLineItemsURL string, agsScopes []string) *AGSClient {
	hc := &http.Client{Timeout: 15 * time.Second}
	return &AGSClient{
		HTTP:         hc,
		TokenURL:     tokenURL,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		LineItemsURL: agsLineItemsURL,
		Scopes:       agsScopes,
	}
}

// ===== Public API =====

// CreateLineItem POSTs a new line item to the platform and returns the created item.
func (c *AGSClient) CreateLineItem(ctx context.Context, li LineItem) (LineItem, error) {
	if c.LineItemsURL == "" {
		return LineItem{}, errors.New("missing LineItemsURL")
	}
	if li.ScoreMaximum <= 0 {
		return LineItem{}, errors.New("scoreMaximum required and > 0")
	}
	tok, err := c.fetchToken(ctx, neededScope(c.Scopes, "https://purl.imsglobal.org/spec/lti-ags/scope/lineitem"))
	if err != nil {
		return LineItem{}, err
	}
	body, _ := json.Marshal(li)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.LineItemsURL, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/vnd.ims.lis.v2.lineitem+json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return LineItem{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return LineItem{}, httpErr("create line item", resp)
	}
	var out LineItem
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return LineItem{}, err
	}
	return out, nil
}

// ListLineItems GETs line items (optionally filter by resourceId, resourceLinkId).
func (c *AGSClient) ListLineItems(ctx context.Context, resourceID, resourceLinkID string, limit, page int) ([]LineItem, error) {
	if c.LineItemsURL == "" {
		return nil, errors.New("missing LineItemsURL")
	}
	tok, err := c.fetchToken(ctx, neededScope(c.Scopes, "https://purl.imsglobal.org/spec/lti-ags/scope/lineitem.readonly", "https://purl.imsglobal.org/spec/lti-ags/scope/lineitem"))
	if err != nil {
		return nil, err
	}
	u, _ := url.Parse(c.LineItemsURL)
	q := u.Query()
	if resourceID != "" {
		q.Set("resource_id", resourceID)
	}
	if resourceLinkID != "" {
		q.Set("resource_link_id", resourceLinkID)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	u.RawQuery = q.Encode()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/vnd.ims.lis.v2.lineitemcontainer+json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, httpErr("list line items", resp)
	}
	var out []LineItem
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteLineItem removes a line item by its absolute item URL (li.ID).
func (c *AGSClient) DeleteLineItem(ctx context.Context, lineItemURL string) error {
	if lineItemURL == "" {
		return errors.New("lineItemURL required")
	}
	tok, err := c.fetchToken(ctx, neededScope(c.Scopes, "https://purl.imsglobal.org/spec/lti-ags/scope/lineitem"))
	if err != nil {
		return err
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodDelete, lineItemURL, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode/100 != 2 {
		return httpErr("delete line item", resp)
	}
	return nil
}

// PostScore posts (upserts) a score to the Scores container of a line item.
// lineItemURL is the absolute item URL from the platform (li.ID). Scores endpoint is "{lineItemURL}/scores".
func (c *AGSClient) PostScore(ctx context.Context, lineItemURL string, s Score) error {
	if lineItemURL == "" {
		return errors.New("lineItemURL required")
	}
	if s.UserID == "" {
		return errors.New("score.userId required")
	}
	if s.Timestamp == "" {
		s.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if s.ActivityProgress == "" {
		s.ActivityProgress = "Completed"
	}
	if s.GradingProgress == "" {
		s.GradingProgress = "FullyGraded"
	}
	tok, err := c.fetchToken(ctx, neededScope(c.Scopes, "https://purl.imsglobal.org/spec/lti-ags/scope/score"))
	if err != nil {
		return err
	}
	u := strings.TrimRight(lineItemURL, "/") + "/scores"
	body, _ := json.Marshal(s)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/vnd.ims.lis.v1.score+json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusNoContent {
		return httpErr("post score", resp)
	}
	return nil
}

// GetResults reads Results for a line item. Optionally filter by userId.
func (c *AGSClient) GetResults(ctx context.Context, lineItemURL, userID string, limit, page int) ([]Result, error) {
	if lineItemURL == "" {
		return nil, errors.New("lineItemURL required")
	}
	tok, err := c.fetchToken(ctx, neededScope(c.Scopes, "https://purl.imsglobal.org/spec/lti-ags/scope/result.readonly"))
	if err != nil {
		return nil, err
	}
	u, _ := url.Parse(strings.TrimRight(lineItemURL, "/") + "/results")
	q := u.Query()
	if userID != "" {
		q.Set("user_id", userID)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	u.RawQuery = q.Encode()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/vnd.ims.lis.v2.resultcontainer+json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, httpErr("get results", resp)
	}
	var out []Result
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// ===== Token (MVP: client_credentials) =====

type tokenResp struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in,omitempty"`
	TokenType   string `json:"token_type,omitempty"`
	Scope       string `json:"scope,omitempty"`
}

func (c *AGSClient) fetchToken(ctx context.Context, scope string) (string, error) {
	if c.TokenURL == "" || c.ClientID == "" || c.ClientSecret == "" {
		return "", errors.New("missing TokenURL/ClientID/ClientSecret")
	}
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	if scope != "" {
		form.Set("scope", scope)
	}
	form.Set("client_id", c.ClientID)
	form.Set("client_secret", c.ClientSecret)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.TokenURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return "", httpErr("fetch token", resp)
	}
	var tr tokenResp
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", err
	}
	if tr.AccessToken == "" {
		return "", errors.New("empty access_token in token response")
	}
	return tr.AccessToken, nil
}

// Choose the first scope the platform granted that matches our desired set.
func neededScope(platformScopes []string, preferred ...string) string {
	pset := make(map[string]struct{}, len(platformScopes))
	for _, s := range platformScopes {
		pset[s] = struct{}{}
	}
	for _, want := range preferred {
		if _, ok := pset[want]; ok {
			return want
		}
	}
	// Fallback: return empty (some platforms ignore scope param for client_credentials)
	return ""
}

// Uniform HTTP error helper.
func httpErr(op string, resp *http.Response) error {
	return fmt.Errorf("%s: platform returned %s", op, resp.Status)
}
