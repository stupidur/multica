package handler

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestHandleLarkCardCallbackCreatesComment(t *testing.T) {
	_, tenantKey, openID, issueID := seedLarkCardCallbackFixture(t)

	req := newRequest("POST", "/api/lark/card/callback", map[string]any{
		"tenant_key": tenantKey,
		"open_id":    openID,
		"action": map[string]any{
			"value": map[string]any{
				"action":       "reply",
				"workspace_id": testWorkspaceID,
				"issue_id":     issueID,
			},
			"form_value": map[string]any{
				larkCardReplyFieldName: "Reply from Feishu",
			},
		},
	})
	rr := httptest.NewRecorder()

	testHandler.HandleLarkCardCallback(rr, req)

	if rr.Code != 200 {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var count int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM comment WHERE issue_id = $1 AND content = $2`, issueID, "Reply from Feishu").Scan(&count); err != nil {
		t.Fatalf("count comment: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 lark callback comment, got %d", count)
	}
}

func TestHandleLarkCardCallbackMarksIssueDone(t *testing.T) {
	_, tenantKey, openID, issueID := seedLarkCardCallbackFixture(t)

	req := newRequest("POST", "/api/lark/card/callback", map[string]any{
		"tenant_key": tenantKey,
		"open_id":    openID,
		"message_id": "om_test_message",
		"action": map[string]any{
			"value": map[string]any{
				"action":       "complete",
				"workspace_id": testWorkspaceID,
				"issue_id":     issueID,
				"body":         "Original comment",
			},
		},
	})
	rr := httptest.NewRecorder()

	testHandler.HandleLarkCardCallback(rr, req)

	if rr.Code != 200 {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var status string
	if err := testPool.QueryRow(context.Background(), `SELECT status FROM issue WHERE id = $1`, issueID).Scan(&status); err != nil {
		t.Fatalf("load issue: %v", err)
	}
	if status != "done" {
		t.Fatalf("expected issue status done, got %q", status)
	}
}

func TestHandleLarkCardCallbackSchema2CreatesComment(t *testing.T) {
	_, tenantKey, openID, issueID := seedLarkCardCallbackFixture(t)

	req := newRequest("POST", "/api/lark/card/callback", map[string]any{
		"schema": "2.0",
		"header": map[string]any{
			"token":      testHandler.cfg.LarkVerificationToken,
			"tenant_key": tenantKey,
		},
		"event": map[string]any{
			"operator": map[string]any{
				"open_id": openID,
			},
			"action": map[string]any{
				"value": map[string]any{
					"action":       "reply",
					"workspace_id": testWorkspaceID,
					"issue_id":     issueID,
				},
				"form_value": map[string]any{
					larkCardReplyFieldName: "Reply from schema 2.0",
				},
			},
		},
	})
	rr := httptest.NewRecorder()

	testHandler.HandleLarkCardCallback(rr, req)

	if rr.Code != 200 {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var count int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM comment WHERE issue_id = $1 AND content = $2`, issueID, "Reply from schema 2.0").Scan(&count); err != nil {
		t.Fatalf("count comment: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 schema 2.0 callback comment, got %d", count)
	}
}

func TestHandleLarkCardCallbackSchema2CreatesCommentFromInputValue(t *testing.T) {
	_, tenantKey, openID, issueID := seedLarkCardCallbackFixture(t)

	req := newRequest("POST", "/api/lark/card/callback", map[string]any{
		"schema": "2.0",
		"header": map[string]any{
			"token":      testHandler.cfg.LarkVerificationToken,
			"tenant_key": tenantKey,
		},
		"event": map[string]any{
			"operator": map[string]any{
				"open_id": openID,
			},
			"action": map[string]any{
				"value": map[string]any{
					"action":       "reply",
					"workspace_id": testWorkspaceID,
					"issue_id":     issueID,
				},
				"input_value": "Reply from input_value",
			},
		},
	})
	rr := httptest.NewRecorder()

	testHandler.HandleLarkCardCallback(rr, req)

	if rr.Code != 200 {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var count int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM comment WHERE issue_id = $1 AND content = $2`, issueID, "Reply from input_value").Scan(&count); err != nil {
		t.Fatalf("count comment: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 schema 2.0 input_value comment, got %d", count)
	}
}

func TestHandleLarkCardCallbackSchema2MarksIssueDone(t *testing.T) {
	_, tenantKey, openID, issueID := seedLarkCardCallbackFixture(t)

	req := newRequest("POST", "/api/lark/card/callback", map[string]any{
		"schema": "2.0",
		"header": map[string]any{
			"token":      testHandler.cfg.LarkVerificationToken,
			"tenant_key": tenantKey,
		},
		"event": map[string]any{
			"operator": map[string]any{
				"open_id": openID,
			},
			"context": map[string]any{
				"open_message_id": "om_test_message",
			},
			"action": map[string]any{
				"value": map[string]any{
					"action":       "complete",
					"workspace_id": testWorkspaceID,
					"issue_id":     issueID,
					"body":         "Original comment",
				},
			},
		},
	})
	rr := httptest.NewRecorder()

	testHandler.HandleLarkCardCallback(rr, req)

	if rr.Code != 200 {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var status string
	if err := testPool.QueryRow(context.Background(), `SELECT status FROM issue WHERE id = $1`, issueID).Scan(&status); err != nil {
		t.Fatalf("load issue: %v", err)
	}
	if status != "done" {
		t.Fatalf("expected issue status done, got %q", status)
	}
}

func seedLarkCardCallbackFixture(t *testing.T) (tenantID string, tenantKey string, openID string, issueID string) {
	t.Helper()
	ctx := context.Background()
	stamp := time.Now().UnixNano()
	tenantKey = fmt.Sprintf("tenant-key-%d", stamp)
	openID = fmt.Sprintf("ou_test_%d", stamp)
	if err := testPool.QueryRow(ctx, `
		INSERT INTO lark_tenant (tenant_key, name)
		VALUES ($1, $2)
		RETURNING id
	`, tenantKey, fmt.Sprintf("Lark Tenant %d", stamp)).Scan(&tenantID); err != nil {
		t.Fatalf("insert lark tenant: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM lark_tenant WHERE id = $1`, tenantID)
	})
	if _, err := testPool.Exec(ctx, `
		INSERT INTO user_identity (user_id, provider, tenant_id, external_user_id)
		VALUES ($1, 'lark', $2, $3)
	`, testUserID, tenantID, openID); err != nil {
		t.Fatalf("insert user identity: %v", err)
	}
	var issueUUID pgtype.UUID
	if err := testPool.QueryRow(ctx, `
		INSERT INTO issue (workspace_id, title, status, priority, creator_type, creator_id, number, position)
		VALUES ($1, $2, 'todo', 'medium', 'member', $3, $4, 1)
		RETURNING id
	`, testWorkspaceID, fmt.Sprintf("Lark Card Issue %d", stamp), testUserID, int(stamp%1000000)).Scan(&issueUUID); err != nil {
		t.Fatalf("insert issue: %v", err)
	}
	issueID = uuidToString(issueUUID)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM comment WHERE issue_id = $1`, issueID)
		_, _ = testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1`, issueID)
		_, _ = testPool.Exec(context.Background(), `DELETE FROM user_identity WHERE provider = 'lark' AND tenant_id = $1 AND external_user_id = $2`, tenantID, openID)
	})
	return tenantID, tenantKey, openID, issueID
}
