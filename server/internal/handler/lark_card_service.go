package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const (
	larkTenantAccessTokenURL = "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal"
	larkMessageCreateURL     = "https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=open_id"
	larkMessageUpdateURL     = "https://open.feishu.cn/open-apis/im/v1/messages/%s"
	larkCardReplyFieldName   = "reply_text"
	larkNotificationProvider = "lark"
)

type LarkInboxCardRequest struct {
	RecipientUserID string
	TenantID        string
	WorkspaceID     string
	IssueID         string
	IssueStatus     string
	Title           string
	Body            string
}

type larkTenantAccessTokenResponse struct {
	Code              int    `json:"code"`
	Msg               string `json:"msg"`
	TenantAccessToken string `json:"tenant_access_token"`
}

type larkMessageCreateResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		MessageID string `json:"message_id"`
	} `json:"data"`
}

type LarkCardService struct {
	queries           *db.Queries
	cfg               Config
	httpClient        *http.Client
	tenantTokenURL    string
	messageCreateURL  string
	messageUpdateURLf string
}

func NewLarkCardService(queries *db.Queries, cfg Config) *LarkCardService {
	return &LarkCardService{
		queries:           queries,
		cfg:               cfg,
		httpClient:        http.DefaultClient,
		tenantTokenURL:    larkTenantAccessTokenURL,
		messageCreateURL:  larkMessageCreateURL,
		messageUpdateURLf: larkMessageUpdateURL,
	}
}

func (s *LarkCardService) Enabled() bool {
	return strings.TrimSpace(s.cfg.LarkAppID) != "" && strings.TrimSpace(s.cfg.LarkAppSecret) != ""
}

func (s *LarkCardService) AvailableForUser(ctx context.Context, userID, tenantID string) bool {
	if !s.Enabled() || strings.TrimSpace(userID) == "" || strings.TrimSpace(tenantID) == "" {
		return false
	}
	identity, err := s.queries.GetUserIdentityByUserProviderTenant(ctx, db.GetUserIdentityByUserProviderTenantParams{
		UserID:   parseUUID(userID),
		Provider: larkNotificationProvider,
		TenantID: parseUUID(tenantID),
	})
	if err != nil {
		return false
	}
	return strings.TrimSpace(identity.ExternalUserID) != ""
}

func (s *LarkCardService) SendInboxCard(ctx context.Context, req LarkInboxCardRequest) error {
	if !s.Enabled() || strings.TrimSpace(req.IssueID) == "" {
		return nil
	}
	if !s.isCardNotificationEnabled(ctx, req.WorkspaceID, req.RecipientUserID) {
		return nil
	}
	issue, workspace, projectName := s.loadCardContext(ctx, req.WorkspaceID, req.IssueID)
	tenantID := strings.TrimSpace(req.TenantID)
	if tenantID == "" && workspace.HomeTenantID.Valid {
		tenantID = util.UUIDToString(workspace.HomeTenantID)
	}
	if tenantID == "" {
		return nil
	}
	identity, err := s.queries.GetUserIdentityByUserProviderTenant(ctx, db.GetUserIdentityByUserProviderTenantParams{
		UserID:   parseUUID(req.RecipientUserID),
		Provider: larkNotificationProvider,
		TenantID: parseUUID(tenantID),
	})
	if err != nil || strings.TrimSpace(identity.ExternalUserID) == "" {
		return nil
	}
	recipient, err := s.queries.GetUser(ctx, parseUUID(req.RecipientUserID))
	if err != nil {
		return nil
	}
	content, err := json.Marshal(s.buildInboxCard(larkCardViewData{
		WorkspaceID:   req.WorkspaceID,
		WorkspaceSlug: workspace.Slug,
		IssueID:       req.IssueID,
		IssueTitle:    firstNonEmptyString(issue.Title, req.Title),
		IssueStatus:   firstNonEmptyString(issue.Status, req.IssueStatus),
		ProjectName:   projectName,
		CommentBody:   req.Body,
		IssueURL:      s.issueURL(workspace.Slug, req.IssueID),
		Language:      recipientLanguage(recipient),
	}))
	if err != nil {
		return err
	}

	_, err = s.sendInteractiveMessage(ctx, identity.ExternalUserID, content)
	return err
}

func (s *LarkCardService) UpdateCardCompleted(ctx context.Context, messageID string, req LarkInboxCardRequest) error {
	if !s.Enabled() || strings.TrimSpace(messageID) == "" || strings.TrimSpace(req.IssueID) == "" {
		return nil
	}
	issue, workspace, projectName := s.loadCardContext(ctx, req.WorkspaceID, req.IssueID)
	recipient, err := s.queries.GetUser(ctx, parseUUID(req.RecipientUserID))
	if err != nil {
		return nil
	}
	content, err := json.Marshal(s.buildCompletedCard(larkCardViewData{
		WorkspaceID:   req.WorkspaceID,
		WorkspaceSlug: workspace.Slug,
		IssueID:       req.IssueID,
		IssueTitle:    firstNonEmptyString(issue.Title, req.Title),
		IssueStatus:   "done",
		ProjectName:   projectName,
		CommentBody:   req.Body,
		IssueURL:      s.issueURL(workspace.Slug, req.IssueID),
		Language:      recipientLanguage(recipient),
	}))
	if err != nil {
		return err
	}
	return s.updateInteractiveMessage(ctx, messageID, content)
}

func (s *LarkCardService) UpdateCardReplied(ctx context.Context, messageID string, req LarkInboxCardRequest) error {
	if !s.Enabled() || strings.TrimSpace(messageID) == "" || strings.TrimSpace(req.IssueID) == "" {
		return nil
	}
	issue, workspace, projectName := s.loadCardContext(ctx, req.WorkspaceID, req.IssueID)
	recipient, err := s.queries.GetUser(ctx, parseUUID(req.RecipientUserID))
	if err != nil {
		return nil
	}
	content, err := json.Marshal(s.buildRepliedCard(larkCardViewData{
		WorkspaceID:   req.WorkspaceID,
		WorkspaceSlug: workspace.Slug,
		IssueID:       req.IssueID,
		IssueTitle:    firstNonEmptyString(issue.Title, req.Title),
		IssueStatus:   firstNonEmptyString(issue.Status, req.IssueStatus),
		ProjectName:   projectName,
		CommentBody:   req.Body,
		IssueURL:      s.issueURL(workspace.Slug, req.IssueID),
		Language:      recipientLanguage(recipient),
	}))
	if err != nil {
		return err
	}
	return s.updateInteractiveMessage(ctx, messageID, content)
}

func (s *LarkCardService) ResolveUserByOpenID(ctx context.Context, tenantKey, openID string) (db.User, db.LarkTenant, error) {
	tenant, err := s.queries.GetLarkTenantByKey(ctx, strings.TrimSpace(tenantKey))
	if err != nil {
		return db.User{}, db.LarkTenant{}, err
	}
	identity, err := s.queries.GetUserIdentityByProviderTenantExternal(ctx, db.GetUserIdentityByProviderTenantExternalParams{
		Provider:       larkNotificationProvider,
		TenantID:       tenant.ID,
		ExternalUserID: strings.TrimSpace(openID),
	})
	if err != nil {
		return db.User{}, db.LarkTenant{}, err
	}
	user, err := s.queries.GetUser(ctx, identity.UserID)
	if err != nil {
		return db.User{}, db.LarkTenant{}, err
	}
	return user, tenant, nil
}

func (s *LarkCardService) isCardNotificationEnabled(ctx context.Context, workspaceID, userID string) bool {
	pref, err := s.queries.GetNotificationPreference(ctx, db.GetNotificationPreferenceParams{
		WorkspaceID: parseUUID(workspaceID),
		UserID:      parseUUID(userID),
	})
	if err != nil {
		return err == pgx.ErrNoRows
	}
	var prefs map[string]string
	if err := json.Unmarshal(pref.Preferences, &prefs); err != nil {
		return true
	}
	return prefs["lark_card_notifications"] != "muted"
}

type larkCardViewData struct {
	WorkspaceID   string
	WorkspaceSlug string
	IssueID       string
	IssueTitle    string
	IssueStatus   string
	ProjectName   string
	CommentBody   string
	IssueURL      string
	Language      string
}

type larkCardCopy struct {
	EmptyBody        string
	ReplyPlaceholder string
	SubmitReply      string
	MarkDone         string
	OpenIssue        string
	CompletedSynced  string
	ReplySynced      string
}

func (s *LarkCardService) buildInboxCard(data larkCardViewData) map[string]any {
	copy := larkCardCopyForLanguage(data.Language)
	body := strings.TrimSpace(stripLarkMentionMarkdown(data.CommentBody))
	if body == "" {
		body = copy.EmptyBody
	}
	return map[string]any{
		"config": map[string]any{
			"wide_screen_mode": true,
			"enable_forward":   true,
			"update_multi":     true,
		},
		"header": map[string]any{
			"template": larkCardTemplateForStatus(data.IssueStatus),
			"title": map[string]any{
				"tag":     "plain_text",
				"content": firstNonEmptyString(data.ProjectName, data.IssueTitle, "Multica"),
			},
		},
		"elements": []map[string]any{
			{
				"tag": "div",
				"text": map[string]any{
					"tag":     "lark_md",
					"content": escapeLarkMarkdown(body),
				},
			},
			{
				"tag": "action",
				"actions": []map[string]any{
					{
						"tag":  "input",
						"name": larkCardReplyFieldName,
						"value": map[string]any{
							"action":       "reply",
							"workspace_id": data.WorkspaceID,
							"issue_id":     data.IssueID,
							"body":         body,
						},
						"placeholder": map[string]any{
							"tag":     "plain_text",
							"content": copy.ReplyPlaceholder,
						},
						"max_length": 1000,
						"width":      "fill",
					},
				},
			},
			{
				"tag": "action",
				"actions": []map[string]any{
					{
						"tag":  "button",
						"type": "default",
						"text": map[string]any{"tag": "plain_text", "content": copy.MarkDone},
						"value": map[string]any{
							"action":       "complete",
							"workspace_id": data.WorkspaceID,
							"issue_id":     data.IssueID,
							"body":         body,
						},
					},
					{
						"tag":       "button",
						"text":      map[string]any{"tag": "plain_text", "content": copy.OpenIssue},
						"multi_url": larkCardMultiURL(data.IssueURL),
					},
				},
			},
		},
	}
}

func (s *LarkCardService) buildCompletedCard(data larkCardViewData) map[string]any {
	copy := larkCardCopyForLanguage(data.Language)
	body := strings.TrimSpace(stripLarkMentionMarkdown(data.CommentBody))
	if body == "" {
		body = copy.EmptyBody
	}
	return map[string]any{
		"config": map[string]any{
			"wide_screen_mode": true,
			"enable_forward":   true,
			"update_multi":     true,
		},
		"header": map[string]any{
			"template": "green",
			"title": map[string]any{
				"tag":     "plain_text",
				"content": firstNonEmptyString(data.ProjectName, data.IssueTitle, "Multica"),
			},
		},
		"elements": []map[string]any{
			{
				"tag": "div",
				"text": map[string]any{
					"tag":     "lark_md",
					"content": escapeLarkMarkdown(body),
				},
			},
			{
				"tag": "note",
				"elements": []map[string]any{{
					"tag":     "plain_text",
					"content": copy.CompletedSynced,
				}},
			},
			{
				"tag": "action",
				"actions": []map[string]any{{
					"tag":       "button",
					"text":      map[string]any{"tag": "plain_text", "content": copy.OpenIssue},
					"multi_url": larkCardMultiURL(data.IssueURL),
				}},
			},
		},
	}
}

func (s *LarkCardService) buildRepliedCard(data larkCardViewData) map[string]any {
	copy := larkCardCopyForLanguage(data.Language)
	body := strings.TrimSpace(stripLarkMentionMarkdown(data.CommentBody))
	if body == "" {
		body = copy.EmptyBody
	}
	return map[string]any{
		"config": map[string]any{
			"wide_screen_mode": true,
			"enable_forward":   true,
			"update_multi":     true,
		},
		"header": map[string]any{
			"template": larkCardTemplateForStatus(data.IssueStatus),
			"title": map[string]any{
				"tag":     "plain_text",
				"content": firstNonEmptyString(data.ProjectName, data.IssueTitle, "Multica"),
			},
		},
		"elements": []map[string]any{
			{
				"tag": "div",
				"text": map[string]any{
					"tag":     "lark_md",
					"content": escapeLarkMarkdown(body),
				},
			},
			{
				"tag": "note",
				"elements": []map[string]any{{
					"tag":     "plain_text",
					"content": copy.ReplySynced,
				}},
			},
			{
				"tag": "action",
				"actions": []map[string]any{{
					"tag":       "button",
					"text":      map[string]any{"tag": "plain_text", "content": copy.OpenIssue},
					"multi_url": larkCardMultiURL(data.IssueURL),
				}},
			},
		},
	}
}

func (s *LarkCardService) loadCardContext(ctx context.Context, workspaceID, issueID string) (db.Issue, db.Workspace, string) {
	var issue db.Issue
	var workspace db.Workspace
	projectName := ""
	if parsedWorkspaceID, err := util.ParseUUID(workspaceID); err == nil {
		if ws, err := s.queries.GetWorkspace(ctx, parsedWorkspaceID); err == nil {
			workspace = ws
			projectName = ws.Name
		}
		if parsedIssueID, err := util.ParseUUID(issueID); err == nil {
			if loadedIssue, err := s.queries.GetIssueInWorkspace(ctx, db.GetIssueInWorkspaceParams{
				ID:          parsedIssueID,
				WorkspaceID: parsedWorkspaceID,
			}); err == nil {
				issue = loadedIssue
				if issue.ProjectID.Valid {
					if project, err := s.queries.GetProject(ctx, issue.ProjectID); err == nil {
						projectName = project.Title
					}
				}
			}
		}
	}
	return issue, workspace, projectName
}

func (s *LarkCardService) issueURL(workspaceSlug, issueID string) string {
	path := fmt.Sprintf("/%s/issues/%s", strings.TrimSpace(workspaceSlug), strings.TrimSpace(issueID))
	if strings.TrimSpace(s.cfg.PublicURL) == "" {
		return path
	}
	return strings.TrimRight(s.cfg.PublicURL, "/") + path
}

func (s *LarkCardService) sendInteractiveMessage(ctx context.Context, openID string, content []byte) (string, error) {
	token, err := s.fetchTenantAccessToken(ctx)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(map[string]any{
		"receive_id": openID,
		"msg_type":   "interactive",
		"content":    string(content),
		"uuid":       fmt.Sprintf("multica-%d", time.Now().UnixNano()),
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.messageCreateURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var payload larkMessageCreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK || payload.Code != 0 {
		return "", fmt.Errorf("lark message send failed: status=%d code=%d msg=%s", resp.StatusCode, payload.Code, payload.Msg)
	}
	return payload.Data.MessageID, nil
}

func (s *LarkCardService) updateInteractiveMessage(ctx context.Context, messageID string, content []byte) error {
	token, err := s.fetchTenantAccessToken(ctx)
	if err != nil {
		return err
	}
	body, err := json.Marshal(map[string]any{"content": string(content)})
	if err != nil {
		return err
	}
	url := fmt.Sprintf(s.messageUpdateURLf, messageID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var payload struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK || payload.Code != 0 {
		return fmt.Errorf("lark message update failed: status=%d code=%d msg=%s", resp.StatusCode, payload.Code, payload.Msg)
	}
	return nil
}

func (s *LarkCardService) fetchTenantAccessToken(ctx context.Context) (string, error) {
	body, err := json.Marshal(map[string]string{
		"app_id":     s.cfg.LarkAppID,
		"app_secret": s.cfg.LarkAppSecret,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tenantTokenURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var payload larkTenantAccessTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK || payload.Code != 0 || strings.TrimSpace(payload.TenantAccessToken) == "" {
		return "", fmt.Errorf("lark tenant token failed: status=%d code=%d msg=%s", resp.StatusCode, payload.Code, payload.Msg)
	}
	return payload.TenantAccessToken, nil
}

func stripLarkMentionMarkdown(text string) string {
	return util.MentionRe.ReplaceAllStringFunc(text, func(match string) string {
		parts := util.MentionRe.FindStringSubmatch(match)
		if len(parts) < 3 {
			return match
		}
		if parts[2] == "issue" {
			return parts[1]
		}
		return "@" + parts[1]
	})
}

func escapeLarkMarkdown(text string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return replacer.Replace(strings.TrimSpace(text))
}

func recipientLanguage(user db.User) string {
	if !user.Language.Valid {
		return ""
	}
	return strings.TrimSpace(user.Language.String)
}

func larkCardCopyForLanguage(language string) larkCardCopy {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "zh") {
		return larkCardCopy{
			EmptyBody:        "无评论内容",
			ReplyPlaceholder: "回复 Multica...",
			SubmitReply:      "发送回复",
			MarkDone:         "标记完成",
			OpenIssue:        "打开问题",
			CompletedSynced:  "已在飞书标记完成，并同步到 Multica。",
			ReplySynced:      "已回复，并同步到 Multica。",
		}
	}
	return larkCardCopy{
		EmptyBody:        "No comment content",
		ReplyPlaceholder: "Reply to Multica...",
		SubmitReply:      "Submit reply",
		MarkDone:         "Mark done",
		OpenIssue:        "Open issue",
		CompletedSynced:  "Completed in Feishu and synced to Multica.",
		ReplySynced:      "Replied and synced to Multica.",
	}
}

func larkCardMultiURL(url string) map[string]any {
	trimmed := strings.TrimSpace(url)
	return map[string]any{
		"url":         trimmed,
		"android_url": trimmed,
		"ios_url":     trimmed,
		"pc_url":      trimmed,
	}
}

func larkCardTemplateForStatus(status string) string {
	switch strings.TrimSpace(status) {
	case "done":
		return "green"
	case "cancelled", "canceled":
		return "grey"
	case "in_progress", "in_review":
		return "orange"
	case "todo", "backlog":
		return "blue"
	default:
		return "blue"
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
