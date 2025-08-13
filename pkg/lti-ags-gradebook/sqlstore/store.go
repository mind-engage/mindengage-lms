package sqlstore

import (
	"database/sql"
	"encoding/json"

	"github.com/mind-engage/mindengage-lms/pkg/lti-ags-gradebook/gradebook"
)

type Store struct{ DB *sql.DB }

func (s *Store) GetExam(id string) (gradebook.Exam, error) {
	var ex gradebook.Exam
	err := s.DB.QueryRow(`SELECT id, title, max_points FROM exams WHERE id=$1`, id).
		Scan(&ex.ID, &ex.Title, &ex.MaxPts)
	return ex, err
}

func (s *Store) GetAttempt(id string) (gradebook.Attempt, error) {
	var a gradebook.Attempt
	err := s.DB.QueryRow(`
		SELECT id, exam_id, user_id, score, submitted_at,
		       platform_issuer, deployment_id, context_id, resource_link_id
		FROM attempts WHERE id=$1`, id).
		Scan(&a.ID, &a.ExamID, &a.UserID, &a.Score, &a.SubmittedAt,
			&a.PlatformIssuer, &a.DeploymentID, &a.ContextID, &a.ResourceLinkID)
	return a, err
}

func (s *Store) GetLatestLinkForContext(issuer, dep, ctxID, rlID string) (gradebook.LTILink, error) {
	var link gradebook.LTILink
	var scopes []byte
	err := s.DB.QueryRow(`
		SELECT platform_issuer, deployment_id, context_id, resource_link_id, lineitems_url, scopes
		FROM lti_links
		WHERE platform_issuer=$1 AND deployment_id=$2 AND context_id=$3 AND resource_link_id=$4
		ORDER BY updated_at DESC LIMIT 1`, issuer, dep, ctxID, rlID).
		Scan(&link.PlatformIssuer, &link.DeploymentID, &link.ContextID, &link.ResourceLinkID, &link.LineItemsURL, &scopes)
	if err != nil {
		return gradebook.LTILink{}, err
	}
	_ = json.Unmarshal(scopes, &link.Scopes)
	return link, nil
}

func (s *Store) UpsertLineItem(li gradebook.GradebookLineItem) (gradebook.GradebookLineItem, error) {
	err := s.DB.QueryRow(`
		INSERT INTO gradebook_lineitems (exam_id, platform_issuer, deployment_id, context_id, resource_link_id, label, score_max, line_item_url)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (exam_id, platform_issuer, deployment_id, context_id, resource_link_id)
		DO UPDATE SET
			label=EXCLUDED.label,
			score_max=EXCLUDED.score_max,
			line_item_url=EXCLUDED.line_item_url,
			updated_at=CURRENT_TIMESTAMP
		RETURNING id`,
		li.ExamID, li.PlatformIssuer, li.DeploymentID, li.ContextID, li.ResourceLinkID, li.Label, li.ScoreMax, li.LineItemURL).
		Scan(&li.ID)
	return li, err
}

func (s *Store) FindLineItem(examID, issuer, dep, ctxID, rlID string) (gradebook.GradebookLineItem, error) {
	var li gradebook.GradebookLineItem
	err := s.DB.QueryRow(`
		SELECT id, exam_id, platform_issuer, deployment_id, context_id, resource_link_id, label, score_max, line_item_url
		FROM gradebook_lineitems
		WHERE exam_id=$1 AND platform_issuer=$2 AND deployment_id=$3 AND context_id=$4 AND resource_link_id=$5`,
		examID, issuer, dep, ctxID, rlID).
		Scan(&li.ID, &li.ExamID, &li.PlatformIssuer, &li.DeploymentID, &li.ContextID, &li.ResourceLinkID, &li.Label, &li.ScoreMax, &li.LineItemURL)
	return li, err
}

func (s *Store) GetPlatformUserID(issuer, localUserID string) (string, error) {
	var sub string
	err := s.DB.QueryRow(`SELECT platform_sub FROM lti_user_map WHERE platform_issuer=$1 AND local_user_id=$2`,
		issuer, localUserID).Scan(&sub)
	return sub, err
}

func (s *Store) MarkSyncPending(attemptID string) error {
	_, err := s.DB.Exec(`
		INSERT INTO grade_sync_status (attempt_id, status, retries, updated_at)
		VALUES ($1,'pending',0,CURRENT_TIMESTAMP)
		ON CONFLICT (attempt_id)
		DO UPDATE SET status='pending', updated_at=CURRENT_TIMESTAMP`,
		attemptID)
	return err
}

func (s *Store) MarkSyncOK(attemptID string) error {
	_, err := s.DB.Exec(`
		UPDATE grade_sync_status
		   SET status='ok', last_error=NULL, updated_at=CURRENT_TIMESTAMP
		 WHERE attempt_id=$1`, attemptID)
	return err
}

func (s *Store) MarkSyncFailed(attemptID string, lastErr string) error {
	_, err := s.DB.Exec(`
		INSERT INTO grade_sync_status (attempt_id, status, retries, last_error, updated_at)
		VALUES ($1,'failed',1,$2,CURRENT_TIMESTAMP)
		ON CONFLICT (attempt_id)
		DO UPDATE SET
			status='failed',
			retries=grade_sync_status.retries+1,
			last_error=$2,
			updated_at=CURRENT_TIMESTAMP`,
		attemptID, lastErr)
	return err
}

func (s *Store) GetPlatform(issuer string) (gradebook.Platform, error) {
	var p gradebook.Platform
	err := s.DB.QueryRow(`SELECT issuer, client_id, token_url, jwks_url, auth_url FROM lti_platforms WHERE issuer=$1`, issuer).
		Scan(&p.Issuer, &p.ClientID, &p.TokenURL, &p.JWKSURL, &p.AuthURL)
	return p, err
}
