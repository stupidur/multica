package handler

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/mention"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

func (h *Handler) HandleLarkCardCallback(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if challenge := lookupString(payload, "challenge"); challenge != "" {
		if !h.isValidLarkVerificationToken(payload) {
			writeError(w, http.StatusForbidden, "invalid verification token")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"challenge": challenge})
		return
	}
	if !h.isValidLarkVerificationToken(payload) {
		writeLarkCardToast(w, http.StatusOK, "error", "Invalid verification token")
		return
	}

	callbackEvent := larkCardCallbackEvent(payload)
	action, _ := callbackEvent["action"].(map[string]any)
	value, _ := action["value"].(map[string]any)
	actionName := lookupString(value, "action")
	workspaceID := lookupString(value, "workspace_id")
	issueID := lookupString(value, "issue_id")
	bodyText := lookupString(value, "body")
	openID := firstNonEmpty(
		lookupString(callbackEvent, "open_id"),
		lookupNestedString(callbackEvent, "operator", "open_id"),
		lookupNestedString(callbackEvent, "user", "open_id"),
	)
	tenantKey := firstNonEmpty(
		lookupString(payload, "tenant_key"),
		lookupNestedString(payload, "header", "tenant_key"),
		lookupString(callbackEvent, "tenant_key"),
		lookupNestedString(callbackEvent, "operator", "tenant_key"),
	)
	messageID := firstNonEmpty(
		lookupString(callbackEvent, "open_message_id"),
		lookupString(callbackEvent, "message_id"),
		lookupNestedString(callbackEvent, "context", "open_message_id"),
		lookupString(payload, "open_message_id"),
		lookupString(payload, "message_id"),
	)
	if actionName == "" || workspaceID == "" || issueID == "" || openID == "" || tenantKey == "" {
		writeLarkCardToast(w, http.StatusOK, "error", "Missing card callback fields")
		return
	}

	cardService := NewLarkCardService(h.Queries, h.cfg)
	user, tenant, err := cardService.ResolveUserByOpenID(r.Context(), tenantKey, openID)
	if err != nil {
		writeLarkCardToast(w, http.StatusOK, "error", "Failed to resolve Lark user")
		return
	}
	issue, ok := h.loadIssueForLarkCardUser(w, r.Context(), user, workspaceID, issueID)
	if !ok {
		return
	}

	switch actionName {
	case "reply":
		replyText := firstNonEmpty(
			lookupNestedString(action, "form_value", larkCardReplyFieldName),
			lookupString(action, "input_value"),
			lookupString(payload, larkCardReplyFieldName),
		)
		if strings.TrimSpace(replyText) == "" {
			writeLarkCardToast(w, http.StatusOK, "error", "Reply content is required")
			return
		}
		if _, err := h.createCommentFromLarkCard(r.Context(), user.ID, issue, replyText); err != nil {
			slog.Warn("lark card reply failed", append(logger.RequestAttrs(r), "error", err, "issue_id", issueID)...)
			writeLarkCardToast(w, http.StatusOK, "error", "Failed to create comment")
			return
		}
		updateReq := LarkInboxCardRequest{
			RecipientUserID: uuidToString(user.ID),
			TenantID:        uuidToString(tenant.ID),
			WorkspaceID:     workspaceID,
			IssueID:         issueID,
			Body:            bodyText,
			Title:           issue.Title,
			IssueStatus:     issue.Status,
		}
		writeLarkCardToast(w, http.StatusOK, "success", "Reply synced to Multica")
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := cardService.UpdateCardReplied(ctx, messageID, updateReq); err != nil {
				slog.Warn("lark card reply update failed", "error", err, "issue_id", issueID, "message_id", messageID)
			}
		}()
	case "complete":
		if _, err := h.markIssueDoneFromLarkCard(r.Context(), user.ID, issue); err != nil {
			slog.Warn("lark card completion failed", append(logger.RequestAttrs(r), "error", err, "issue_id", issueID)...)
			writeLarkCardToast(w, http.StatusOK, "error", "Failed to complete issue")
			return
		}
		updateReq := LarkInboxCardRequest{
			RecipientUserID: uuidToString(user.ID),
			TenantID:        uuidToString(tenant.ID),
			WorkspaceID:     workspaceID,
			IssueID:         issueID,
			Body:            bodyText,
			Title:           issue.Title,
			IssueStatus:     "done",
		}
		writeLarkCardToast(w, http.StatusOK, "success", "Issue marked done")
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := cardService.UpdateCardCompleted(ctx, messageID, updateReq); err != nil {
				slog.Warn("lark card completion update failed", "error", err, "issue_id", issueID, "message_id", messageID)
			}
		}()
	default:
		writeLarkCardToast(w, http.StatusOK, "error", "Unsupported Lark card action")
	}
}

func (h *Handler) loadIssueForLarkCardUser(w http.ResponseWriter, ctx context.Context, user db.User, workspaceID, issueID string) (db.Issue, bool) {
	if _, err := h.Queries.GetMemberByUserAndWorkspace(ctx, db.GetMemberByUserAndWorkspaceParams{
		UserID:      user.ID,
		WorkspaceID: parseUUID(workspaceID),
	}); err != nil {
		writeLarkCardToast(w, http.StatusOK, "error", "Workspace access denied")
		return db.Issue{}, false
	}
	issue, err := h.Queries.GetIssueInWorkspace(ctx, db.GetIssueInWorkspaceParams{
		ID:          parseUUID(issueID),
		WorkspaceID: parseUUID(workspaceID),
	})
	if err != nil {
		writeLarkCardToast(w, http.StatusOK, "error", "Issue not found")
		return db.Issue{}, false
	}
	return issue, true
}

func writeLarkCardToast(w http.ResponseWriter, status int, toastType, content string) {
	writeJSON(w, status, map[string]any{
		"toast": map[string]string{
			"type":    toastType,
			"content": content,
		},
	})
}

func (h *Handler) createCommentFromLarkCard(ctx context.Context, userID pgtype.UUID, issue db.Issue, content string) (CommentResponse, error) {
	content = strings.TrimSpace(content)
	content = mention.ExpandIssueIdentifiers(ctx, h.Queries, issue.WorkspaceID, content)
	comment, err := h.Queries.CreateComment(ctx, db.CreateCommentParams{
		IssueID:     issue.ID,
		WorkspaceID: issue.WorkspaceID,
		AuthorType:  "member",
		AuthorID:    userID,
		Content:     content,
		Type:        "comment",
	})
	if err != nil {
		return CommentResponse{}, err
	}
	resp := commentToResponse(comment, nil, nil)
	actorID := uuidToString(userID)
	h.publish(protocol.EventCommentCreated, uuidToString(issue.WorkspaceID), "member", actorID, map[string]any{
		"comment":             resp,
		"issue_title":         issue.Title,
		"issue_assignee_type": textToPtr(issue.AssigneeType),
		"issue_assignee_id":   uuidToPtr(issue.AssigneeID),
		"issue_status":        issue.Status,
	})
	h.triggerTasksForComment(ctx, issue, comment, nil, "member", actorID, nil)
	return resp, nil
}

func (h *Handler) markIssueDoneFromLarkCard(ctx context.Context, userID pgtype.UUID, prevIssue db.Issue) (IssueResponse, error) {
	params := db.UpdateIssueParams{
		ID:            prevIssue.ID,
		AssigneeType:  prevIssue.AssigneeType,
		AssigneeID:    prevIssue.AssigneeID,
		StartDate:     prevIssue.StartDate,
		DueDate:       prevIssue.DueDate,
		ParentIssueID: prevIssue.ParentIssueID,
		ProjectID:     prevIssue.ProjectID,
		Status:        pgtype.Text{String: "done", Valid: true},
	}
	issue, err := h.Queries.UpdateIssue(ctx, params)
	if err != nil {
		return IssueResponse{}, err
	}
	prefix := h.getIssuePrefix(ctx, issue.WorkspaceID)
	resp := issueToResponse(issue, prefix)
	prevStartDate := dateToPtr(prevIssue.StartDate)
	prevDueDate := dateToPtr(prevIssue.DueDate)
	actorID := uuidToString(userID)
	h.publish(protocol.EventIssueUpdated, uuidToString(issue.WorkspaceID), "member", actorID, map[string]any{
		"issue":               resp,
		"assignee_changed":    false,
		"status_changed":      prevIssue.Status != issue.Status,
		"priority_changed":    false,
		"start_date_changed":  false,
		"due_date_changed":    false,
		"description_changed": false,
		"title_changed":       false,
		"prev_title":          prevIssue.Title,
		"prev_assignee_type":  textToPtr(prevIssue.AssigneeType),
		"prev_assignee_id":    uuidToPtr(prevIssue.AssigneeID),
		"prev_status":         prevIssue.Status,
		"prev_priority":       prevIssue.Priority,
		"prev_start_date":     prevStartDate,
		"prev_due_date":       prevDueDate,
		"prev_description":    textToPtr(prevIssue.Description),
		"creator_type":        prevIssue.CreatorType,
		"creator_id":          uuidToString(prevIssue.CreatorID),
	})
	if prevIssue.Status != "done" && issue.Status == "done" {
		h.notifyParentOfChildDone(ctx, prevIssue, issue, "member", actorID)
	}
	return resp, nil
}

func (h *Handler) isValidLarkVerificationToken(payload map[string]any) bool {
	expected := strings.TrimSpace(h.cfg.LarkVerificationToken)
	if expected == "" {
		return true
	}
	return firstNonEmpty(
		lookupString(payload, "token"),
		lookupNestedString(payload, "header", "token"),
	) == expected
}

func larkCardCallbackEvent(payload map[string]any) map[string]any {
	if lookupString(payload, "schema") == "2.0" {
		if event, _ := payload["event"].(map[string]any); event != nil {
			return event
		}
	}
	return payload
}

func lookupString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return strings.TrimSpace(v)
}

func lookupNestedString(m map[string]any, path ...string) string {
	current := m
	for i, segment := range path {
		if i == len(path)-1 {
			return lookupString(current, segment)
		}
		next, _ := current[segment].(map[string]any)
		if next == nil {
			return ""
		}
		current = next
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
