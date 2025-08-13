package gradebook_test

import (
	"fmt"
	"testing"
	"time"

	gradebook "github.com/mind-engage/mindengage-lms/pkg/lti-ags-gradebook/gradebook"
)

/* ---------------- In-memory fakes that satisfy gradebook.Store & gradebook.AGSClient ---------------- */

type fakeStore struct {
	exams       map[string]gradebook.Exam
	attempts    map[string]gradebook.Attempt
	links       map[string]gradebook.LTILink           // key: issuer|dep|ctx|rl
	lineitems   map[string]gradebook.GradebookLineItem // key: exam|issuer|dep|ctx|rl
	lineitemSeq int64
	userMap     map[string]string // key: issuer|localUserID => platformSub
	syncStatus  map[string]struct {
		status, lastErr string
		retries         int
	}
	platformByIssr map[string]gradebook.Platform
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		exams:     map[string]gradebook.Exam{},
		attempts:  map[string]gradebook.Attempt{},
		links:     map[string]gradebook.LTILink{},
		lineitems: map[string]gradebook.GradebookLineItem{},
		userMap:   map[string]string{},
		syncStatus: map[string]struct {
			status, lastErr string
			retries         int
		}{},
		platformByIssr: map[string]gradebook.Platform{},
	}
}

func key(parts ...string) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s", parts[0], parts[1], parts[2], parts[3], parts[4])
}

func (s *fakeStore) GetExam(id string) (gradebook.Exam, error) {
	ex, ok := s.exams[id]
	if !ok {
		return gradebook.Exam{}, fmt.Errorf("exam %q not found", id)
	}
	return ex, nil
}

func (s *fakeStore) GetAttempt(id string) (gradebook.Attempt, error) {
	a, ok := s.attempts[id]
	if !ok {
		return gradebook.Attempt{}, fmt.Errorf("attempt %q not found", id)
	}
	return a, nil
}

func (s *fakeStore) GetLatestLinkForContext(issuer, dep, ctxID, rlID string) (gradebook.LTILink, error) {
	k := fmt.Sprintf("%s|%s|%s|%s", issuer, dep, ctxID, rlID)
	l, ok := s.links[k]
	if !ok {
		return gradebook.LTILink{}, fmt.Errorf("link not found: %s", k)
	}
	return l, nil
}

func (s *fakeStore) UpsertLineItem(li gradebook.GradebookLineItem) (gradebook.GradebookLineItem, error) {
	k := key(li.ExamID, li.PlatformIssuer, li.DeploymentID, li.ContextID, li.ResourceLinkID)
	existing, ok := s.lineitems[k]
	if ok {
		// update
		existing.Label = li.Label
		existing.ScoreMax = li.ScoreMax
		existing.LineItemURL = li.LineItemURL
		s.lineitems[k] = existing
		return existing, nil
	}
	s.lineitemSeq++
	li.ID = s.lineitemSeq
	s.lineitems[k] = li
	return li, nil
}

func (s *fakeStore) FindLineItem(examID, issuer, dep, ctxID, rlID string) (gradebook.GradebookLineItem, error) {
	k := key(examID, issuer, dep, ctxID, rlID)
	li, ok := s.lineitems[k]
	if !ok {
		return gradebook.GradebookLineItem{}, fmt.Errorf("lineitem not found")
	}
	return li, nil
}

func (s *fakeStore) GetPlatformUserID(issuer, localUserID string) (string, error) {
	k := fmt.Sprintf("%s|%s", issuer, localUserID)
	sub, ok := s.userMap[k]
	if !ok {
		return "", fmt.Errorf("mapping not found")
	}
	return sub, nil
}

func (s *fakeStore) MarkSyncPending(attemptID string) error {
	state := s.syncStatus[attemptID]
	state.status = "pending"
	s.syncStatus[attemptID] = state
	return nil
}
func (s *fakeStore) MarkSyncOK(attemptID string) error {
	state := s.syncStatus[attemptID]
	state.status, state.lastErr = "ok", ""
	s.syncStatus[attemptID] = state
	return nil
}
func (s *fakeStore) MarkSyncFailed(attemptID, lastErr string) error {
	state := s.syncStatus[attemptID]
	state.status, state.lastErr, state.retries = "failed", lastErr, state.retries+1
	s.syncStatus[attemptID] = state
	return nil
}

func (s *fakeStore) GetPlatform(issuer string) (gradebook.Platform, error) {
	p, ok := s.platformByIssr[issuer]
	if !ok {
		return gradebook.Platform{}, fmt.Errorf("platform not found")
	}
	return p, nil
}

type fakeAGS struct {
	listed      []gradebook.LineItem
	createdReq  *gradebook.CreateLineItemReq
	createdResp *gradebook.LineItem
	postCalls   int
	postErr     error
}

func (f *fakeAGS) ListLineItems(_ string, _ map[string]string) ([]gradebook.LineItem, error) {
	return f.listed, nil
}
func (f *fakeAGS) CreateLineItem(_ string, req gradebook.CreateLineItemReq) (gradebook.LineItem, error) {
	f.createdReq = &req
	li := gradebook.LineItem{
		ID:             "https://lms.example/lineitems/123",
		Label:          req.Label,
		ScoreMaximum:   req.ScoreMaximum,
		ResourceID:     req.ResourceID,
		ResourceLinkID: req.ResourceLinkID,
	}
	f.createdResp = &li
	return li, nil
}
func (f *fakeAGS) PostScore(_ string, _ gradebook.Score) error {
	f.postCalls++
	return f.postErr
}

/* ------------------------------------------ Tests ------------------------------------------ */

func seedBasic(t *testing.T) (*fakeStore, *fakeAGS, *gradebook.Syncer, string) {
	t.Helper()
	st := newFakeStore()
	ags := &fakeAGS{}
	now := time.Now()
	submittedAt := now

	// Seed exam and attempt
	st.exams["exam-1"] = gradebook.Exam{ID: "exam-1", Title: "Exam One", MaxPts: 100}
	st.attempts["attempt-1"] = gradebook.Attempt{
		ID:             "attempt-1",
		ExamID:         "exam-1",
		UserID:         "u1",
		Score:          80,
		SubmittedAt:    &submittedAt,
		PlatformIssuer: "iss-1",
		DeploymentID:   "dep-1",
		ContextID:      "ctx-1",
		ResourceLinkID: "rl-1",
	}
	// Seed LTI link context with line items collection URL
	st.links["iss-1|dep-1|ctx-1|rl-1"] = gradebook.LTILink{
		PlatformIssuer: "iss-1",
		DeploymentID:   "dep-1",
		ContextID:      "ctx-1",
		ResourceLinkID: "rl-1",
		LineItemsURL:   "https://lms.example/lineitems",
		Scopes:         []string{"lineitem", "score"},
	}
	// User mapping local→platform
	st.userMap["iss-1|u1"] = "platform-sub-123"

	s := gradebook.New(st, ags, time.Now)
	return st, ags, s, "attempt-1"
}

func TestSyncer_CreatesAndPosts(t *testing.T) {
	st, ags, syncer, attemptID := seedBasic(t)

	// No existing line item in store and AGS returns empty list: should create and post.
	if err := syncer.SyncAttempt(attemptID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ags.createdResp == nil {
		t.Fatalf("expected CreateLineItem to be called")
	}
	if ags.postCalls != 1 {
		t.Fatalf("expected 1 PostScore call, got %d", ags.postCalls)
	}

	// Verify line item persisted in store
	k := key("exam-1", "iss-1", "dep-1", "ctx-1", "rl-1")
	if _, ok := st.lineitems[k]; !ok {
		t.Fatalf("expected line item persisted in store")
	}
	if st.syncStatus[attemptID].status != "ok" {
		t.Fatalf("expected sync status ok; got %q", st.syncStatus[attemptID].status)
	}
}

func TestSyncer_UsesExistingLineItem(t *testing.T) {
	_, ags, syncer, attemptID := seedBasic(t)

	// Pretend AGS already has a matching line item
	ags.listed = []gradebook.LineItem{{
		ID:             "https://lms.example/lineitems/exist",
		Label:          "Exam One",
		ScoreMaximum:   100,
		ResourceID:     "exam-1",
		ResourceLinkID: "rl-1",
	}}

	if err := syncer.SyncAttempt(attemptID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should NOT have created a new line item
	if ags.createdResp != nil {
		t.Fatalf("did not expect CreateLineItem to be called")
	}
	if ags.postCalls != 1 {
		t.Fatalf("expected 1 PostScore call, got %d", ags.postCalls)
	}
}

func TestSyncer_FailsWithoutUserMapping(t *testing.T) {
	st, ags, syncer, attemptID := seedBasic(t)
	// Remove user mapping
	delete(st.userMap, "iss-1|u1")

	err := syncer.SyncAttempt(attemptID)
	if err == nil {
		t.Fatalf("expected error without platform user mapping")
	}
	if st.syncStatus[attemptID].status != "failed" {
		t.Fatalf("expected sync status failed; got %q", st.syncStatus[attemptID].status)
	}
	// No score should be posted
	if ags.postCalls != 0 {
		t.Fatalf("expected 0 PostScore calls, got %d", ags.postCalls)
	}
}
