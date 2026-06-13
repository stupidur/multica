package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/analytics"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/logger"
	obsmetrics "github.com/multica-ai/multica/server/internal/metrics"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const (
	larkProvider               = "lark"
	larkAppAccessTokenURL      = "https://open.feishu.cn/open-apis/auth/v3/app_access_token/internal"
	larkOAuthTokenURL          = "https://open.feishu.cn/open-apis/authen/v2/oauth/token"
	larkOIDCUserInfoURL        = "https://open.feishu.cn/open-apis/authen/v1/user_info"
	larkDefaultSyntheticDomain = "lark.multica.local"
)

type LarkLoginRequest struct {
	Code        string `json:"code"`
	RedirectURI string `json:"redirect_uri"`
}

type larkAppAccessTokenResponse struct {
	Code           int    `json:"code"`
	Msg            string `json:"msg"`
	AppAccessToken string `json:"app_access_token"`
	Expire         int    `json:"expire"`
}

type larkOIDCAccessTokenEnvelope struct {
	Code             int    `json:"code"`
	Msg              string `json:"msg"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	AccessToken      string `json:"access_token"`
}

type larkUserInfoEnvelope struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		OpenID    string `json:"open_id"`
		UnionID   string `json:"union_id"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
		Email     string `json:"email"`
		TenantKey string `json:"tenant_key"`
	} `json:"data"`
}

func (h *Handler) LarkLogin(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(h.cfg.LarkAppID) == "" || strings.TrimSpace(h.cfg.LarkAppSecret) == "" {
		writeError(w, http.StatusServiceUnavailable, "Lark login is not configured")
		return
	}

	var req LarkLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Code) == "" {
		writeError(w, http.StatusBadRequest, "code is required")
		return
	}
	redirectURI := strings.TrimSpace(req.RedirectURI)
	if redirectURI == "" {
		redirectURI = strings.TrimSpace(h.cfg.LarkRedirectURI)
	}

	userToken, err := h.fetchLarkUserAccessToken(r.Context(), strings.TrimSpace(req.Code), redirectURI)
	if err != nil {
		slog.Warn("lark login: access token exchange failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusBadGateway, "failed to authenticate with Lark")
		return
	}
	profile, err := h.fetchLarkUserProfile(r.Context(), userToken)
	if err != nil {
		slog.Warn("lark login: user info failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusBadGateway, "failed to load Lark user")
		return
	}
	if strings.TrimSpace(profile.Data.OpenID) == "" || strings.TrimSpace(profile.Data.TenantKey) == "" {
		writeError(w, http.StatusBadGateway, "Lark user info missing tenant identity")
		return
	}

	tenantName := strings.TrimSpace(profile.Data.TenantKey)
	tenant, err := h.Queries.UpsertLarkTenantByKey(r.Context(), db.UpsertLarkTenantByKeyParams{
		TenantKey: strings.TrimSpace(profile.Data.TenantKey),
		Name:      tenantName,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save tenant")
		return
	}

	user, isNew, err := h.findOrCreateUserForLark(r, tenant, profile)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}
	if err := h.ensureLarkTenantAdmin(r.Context(), tenant.ID, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to initialize tenant admin")
		return
	}

	tokenString, err := h.issueJWTForTenant(user, uuidToString(tenant.ID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	if err := auth.SetAuthCookies(w, tokenString); err != nil {
		slog.Warn("failed to set auth cookies", "error", err)
	}
	if h.CFSigner != nil {
		for _, cookie := range h.CFSigner.SignedCookies(time.Now().Add(auth.AuthTokenTTL())) {
			http.SetCookie(w, cookie)
		}
	}
	if isNew {
		evt := analytics.Signup(uuidToString(user.ID), user.Email, signupSourceFromRequest(r))
		evt.Properties["auth_method"] = larkProvider
		evt.Properties["tenant_key"] = profile.Data.TenantKey
		obsmetrics.RecordEvent(h.Analytics, h.Metrics, evt)
	}

	slog.Info("user logged in via lark", append(logger.RequestAttrs(r), "user_id", uuidToString(user.ID), "tenant_id", uuidToString(tenant.ID), "open_id", profile.Data.OpenID)...)
	writeJSON(w, http.StatusOK, LoginResponse{Token: tokenString, User: userToResponse(user)})
}

func (h *Handler) findOrCreateUserForLark(r *http.Request, tenant db.LarkTenant, profile larkUserInfoEnvelope) (db.User, bool, error) {
	identity, err := h.Queries.GetUserIdentityByProviderTenantExternal(r.Context(), db.GetUserIdentityByProviderTenantExternalParams{
		Provider:       larkProvider,
		TenantID:       tenant.ID,
		ExternalUserID: profile.Data.OpenID,
	})
	if err != nil && !isNotFound(err) {
		return db.User{}, false, err
	}
	if err == nil {
		user, userErr := h.Queries.GetUser(r.Context(), identity.UserID)
		if userErr != nil {
			return db.User{}, false, userErr
		}
		updated, updateErr := h.syncUserFromLarkProfile(r.Context(), user, profile)
		if updateErr == nil {
			user = updated
		}
		_, _ = h.upsertLarkIdentity(r.Context(), user.ID, tenant.ID, profile)
		return user, false, nil
	}

	email := strings.ToLower(strings.TrimSpace(profile.Data.Email))
	if email == "" {
		email = fmt.Sprintf("%s@%s", strings.TrimSpace(profile.Data.OpenID), larkDefaultSyntheticDomain)
	}
	user, isNew, userErr := h.findOrCreateUser(r.Context(), email)
	if userErr != nil {
		return db.User{}, false, userErr
	}
	updated, updateErr := h.syncUserFromLarkProfile(r.Context(), user, profile)
	if updateErr == nil {
		user = updated
	}
	if _, err := h.upsertLarkIdentity(r.Context(), user.ID, tenant.ID, profile); err != nil {
		return db.User{}, false, err
	}
	return user, isNew, nil
}

func (h *Handler) syncUserFromLarkProfile(ctx context.Context, user db.User, profile larkUserInfoEnvelope) (db.User, error) {
	name := strings.TrimSpace(profile.Data.Name)
	avatar := strings.TrimSpace(profile.Data.AvatarURL)
	needsUpdate := false
	newName := user.Name
	newAvatar := user.AvatarUrl
	if name != "" && name != user.Name {
		newName = name
		needsUpdate = true
	}
	if avatar != "" && (!user.AvatarUrl.Valid || user.AvatarUrl.String != avatar) {
		newAvatar = pgtype.Text{String: avatar, Valid: true}
		needsUpdate = true
	}
	if !needsUpdate {
		return user, nil
	}
	return h.Queries.UpdateUser(ctx, db.UpdateUserParams{
		ID:        user.ID,
		Name:      newName,
		AvatarUrl: newAvatar,
	})
}

func (h *Handler) upsertLarkIdentity(ctx context.Context, userID, tenantID pgtype.UUID, profile larkUserInfoEnvelope) (db.UserIdentity, error) {
	return h.Queries.UpsertUserIdentity(ctx, db.UpsertUserIdentityParams{
		UserID:         userID,
		Provider:       larkProvider,
		TenantID:       tenantID,
		ExternalUserID: strings.TrimSpace(profile.Data.OpenID),
		UnionID:        ptrToText(optionalStringPtr(profile.Data.UnionID)),
		Email:          ptrToText(optionalStringPtr(strings.ToLower(strings.TrimSpace(profile.Data.Email)))),
	})
}

func (h *Handler) ensureLarkTenantAdmin(ctx context.Context, tenantID, userID pgtype.UUID) error {
	count, err := h.Queries.CountLarkTenantAdmins(ctx, tenantID)
	if err != nil {
		return err
	}
	if count != 0 {
		return nil
	}
	_, err = h.Queries.UpsertLarkTenantAdmin(ctx, db.UpsertLarkTenantAdminParams{
		TenantID: tenantID,
		UserID:   userID,
		Role:     "owner",
	})
	return err
}

func (h *Handler) fetchLarkAppAccessToken(ctx context.Context) (string, error) {
	body, err := json.Marshal(map[string]string{
		"app_id":     h.cfg.LarkAppID,
		"app_secret": h.cfg.LarkAppSecret,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, larkAppAccessTokenURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var payload larkAppAccessTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK || payload.Code != 0 || strings.TrimSpace(payload.AppAccessToken) == "" {
		return "", fmt.Errorf("lark app token failed: status=%d code=%d msg=%s", resp.StatusCode, payload.Code, payload.Msg)
	}
	return payload.AppAccessToken, nil
}

func (h *Handler) fetchLarkUserAccessToken(ctx context.Context, code, redirectURI string) (string, error) {
	payload := map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     strings.TrimSpace(h.cfg.LarkAppID),
		"client_secret": strings.TrimSpace(h.cfg.LarkAppSecret),
		"code":          code,
	}
	if redirectURI != "" {
		payload["redirect_uri"] = redirectURI
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, larkOAuthTokenURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var envelope larkOIDCAccessTokenEnvelope
	if err := json.Unmarshal(rawBody, &envelope); err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK || envelope.Code != 0 || strings.TrimSpace(envelope.AccessToken) == "" {
		return "", fmt.Errorf("lark oauth token failed: status=%d code=%d msg=%s error=%s description=%s body=%s", resp.StatusCode, envelope.Code, envelope.Msg, envelope.Error, envelope.ErrorDescription, string(rawBody))
	}
	return envelope.AccessToken, nil
}

func (h *Handler) fetchLarkUserProfile(ctx context.Context, userToken string) (larkUserInfoEnvelope, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, larkOIDCUserInfoURL, nil)
	if err != nil {
		return larkUserInfoEnvelope{}, err
	}
	req.Header.Set("Authorization", "Bearer "+userToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return larkUserInfoEnvelope{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return larkUserInfoEnvelope{}, err
	}
	var envelope larkUserInfoEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return larkUserInfoEnvelope{}, err
	}
	if resp.StatusCode != http.StatusOK || envelope.Code != 0 {
		return larkUserInfoEnvelope{}, fmt.Errorf("lark user info failed: status=%d code=%d msg=%s", resp.StatusCode, envelope.Code, envelope.Msg)
	}
	return envelope, nil
}

func optionalStringPtr(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}
