package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/auth"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const daemonPairingTTL = 10 * time.Minute

type daemonPairingSessionRecord struct {
	Token          string
	DaemonID       string
	DeviceName     string
	RuntimeName    string
	RuntimeType    string
	RuntimeVersion string
	WorkspaceID    pgtype.UUID
	ApprovedBy     pgtype.UUID
	Status         string
	ApprovedAt     pgtype.Timestamptz
	ClaimedAt      pgtype.Timestamptz
	ExpiresAt      pgtype.Timestamptz
	CreatedAt      pgtype.Timestamptz
	UpdatedAt      pgtype.Timestamptz
}

type DaemonPairingSessionResponse struct {
	Token          string  `json:"token"`
	DaemonID       string  `json:"daemon_id"`
	DeviceName     string  `json:"device_name"`
	RuntimeName    string  `json:"runtime_name"`
	RuntimeType    string  `json:"runtime_type"`
	RuntimeVersion string  `json:"runtime_version"`
	WorkspaceID    *string `json:"workspace_id"`
	Status         string  `json:"status"`
	ApprovedAt     *string `json:"approved_at"`
	ClaimedAt      *string `json:"claimed_at"`
	ExpiresAt      string  `json:"expires_at"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
	LinkURL        *string `json:"link_url,omitempty"`
	DaemonToken    *string `json:"daemon_token,omitempty"`
}

type CreateDaemonPairingSessionRequest struct {
	DaemonID       string `json:"daemon_id"`
	DeviceName     string `json:"device_name"`
	RuntimeName    string `json:"runtime_name"`
	RuntimeType    string `json:"runtime_type"`
	RuntimeVersion string `json:"runtime_version"`
}

type ApproveDaemonPairingSessionRequest struct {
	WorkspaceID string `json:"workspace_id"`
}

func daemonAppBaseURL() string {
	for _, key := range []string{"MULTICA_APP_URL", "FRONTEND_ORIGIN"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return strings.TrimRight(value, "/")
		}
	}
	return "http://localhost:3000"
}

func daemonPairingLinkURL(token string) string {
	base := daemonAppBaseURL()
	return base + "/pair/local?token=" + url.QueryEscape(token)
}

func daemonPairingSessionToResponse(rec daemonPairingSessionRecord, includeLink bool) DaemonPairingSessionResponse {
	resp := DaemonPairingSessionResponse{
		Token:          rec.Token,
		DaemonID:       rec.DaemonID,
		DeviceName:     rec.DeviceName,
		RuntimeName:    rec.RuntimeName,
		RuntimeType:    rec.RuntimeType,
		RuntimeVersion: rec.RuntimeVersion,
		WorkspaceID:    uuidToPtr(rec.WorkspaceID),
		Status:         rec.Status,
		ApprovedAt:     timestampToPtr(rec.ApprovedAt),
		ClaimedAt:      timestampToPtr(rec.ClaimedAt),
		ExpiresAt:      timestampToString(rec.ExpiresAt),
		CreatedAt:      timestampToString(rec.CreatedAt),
		UpdatedAt:      timestampToString(rec.UpdatedAt),
	}
	if includeLink {
		link := daemonPairingLinkURL(rec.Token)
		resp.LinkURL = &link
	}
	return resp
}

func randomDaemonPairingToken() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func (h *Handler) getDaemonPairingSession(ctx context.Context, token string) (daemonPairingSessionRecord, error) {
	if h.DB == nil {
		return daemonPairingSessionRecord{}, fmt.Errorf("database executor is not configured")
	}

	var rec daemonPairingSessionRecord
	err := h.DB.QueryRow(ctx, `
		SELECT
			token,
			daemon_id,
			device_name,
			runtime_name,
			runtime_type,
			runtime_version,
			workspace_id,
			approved_by,
			status,
			approved_at,
			claimed_at,
			expires_at,
			created_at,
			updated_at
		FROM daemon_pairing_session
		WHERE token = $1
	`, token).Scan(
		&rec.Token,
		&rec.DaemonID,
		&rec.DeviceName,
		&rec.RuntimeName,
		&rec.RuntimeType,
		&rec.RuntimeVersion,
		&rec.WorkspaceID,
		&rec.ApprovedBy,
		&rec.Status,
		&rec.ApprovedAt,
		&rec.ClaimedAt,
		&rec.ExpiresAt,
		&rec.CreatedAt,
		&rec.UpdatedAt,
	)
	if err != nil {
		return daemonPairingSessionRecord{}, err
	}

	if rec.Status == "pending" && rec.ExpiresAt.Valid && rec.ExpiresAt.Time.Before(time.Now()) {
		if _, err := h.DB.Exec(ctx, `
			UPDATE daemon_pairing_session
			SET status = 'expired', updated_at = now()
			WHERE token = $1 AND status = 'pending'
		`, token); err == nil {
			rec.Status = "expired"
			rec.UpdatedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
		}
	}

	return rec, nil
}

func (h *Handler) CreateDaemonPairingSession(w http.ResponseWriter, r *http.Request) {
	if h.DB == nil {
		writeError(w, http.StatusInternalServerError, "database executor is not configured")
		return
	}

	var req CreateDaemonPairingSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.DaemonID = strings.TrimSpace(req.DaemonID)
	req.DeviceName = strings.TrimSpace(req.DeviceName)
	req.RuntimeName = strings.TrimSpace(req.RuntimeName)
	req.RuntimeType = strings.TrimSpace(req.RuntimeType)
	req.RuntimeVersion = strings.TrimSpace(req.RuntimeVersion)

	if req.DaemonID == "" {
		writeError(w, http.StatusBadRequest, "daemon_id is required")
		return
	}
	if req.DeviceName == "" {
		writeError(w, http.StatusBadRequest, "device_name is required")
		return
	}
	if req.RuntimeName == "" {
		writeError(w, http.StatusBadRequest, "runtime_name is required")
		return
	}
	if req.RuntimeType == "" {
		writeError(w, http.StatusBadRequest, "runtime_type is required")
		return
	}

	token, err := randomDaemonPairingToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create pairing token")
		return
	}

	expiresAt := time.Now().Add(daemonPairingTTL)
	var rec daemonPairingSessionRecord
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO daemon_pairing_session (
			token,
			daemon_id,
			device_name,
			runtime_name,
			runtime_type,
			runtime_version,
			expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING
			token,
			daemon_id,
			device_name,
			runtime_name,
			runtime_type,
			runtime_version,
			workspace_id,
			approved_by,
			status,
			approved_at,
			claimed_at,
			expires_at,
			created_at,
			updated_at
	`,
		token,
		req.DaemonID,
		req.DeviceName,
		req.RuntimeName,
		req.RuntimeType,
		req.RuntimeVersion,
		expiresAt,
	).Scan(
		&rec.Token,
		&rec.DaemonID,
		&rec.DeviceName,
		&rec.RuntimeName,
		&rec.RuntimeType,
		&rec.RuntimeVersion,
		&rec.WorkspaceID,
		&rec.ApprovedBy,
		&rec.Status,
		&rec.ApprovedAt,
		&rec.ClaimedAt,
		&rec.ExpiresAt,
		&rec.CreatedAt,
		&rec.UpdatedAt,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create pairing session")
		return
	}

	writeJSON(w, http.StatusCreated, daemonPairingSessionToResponse(rec, true))
}

func (h *Handler) GetDaemonPairingSession(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	rec, err := h.getDaemonPairingSession(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusNotFound, "pairing session not found")
		return
	}
	writeJSON(w, http.StatusOK, daemonPairingSessionToResponse(rec, true))
}

func (h *Handler) ApproveDaemonPairingSession(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	rec, err := h.getDaemonPairingSession(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusNotFound, "pairing session not found")
		return
	}
	if rec.Status == "expired" {
		writeError(w, http.StatusBadRequest, "pairing session expired")
		return
	}
	if rec.Status == "claimed" {
		writeError(w, http.StatusBadRequest, "pairing session already claimed")
		return
	}
	if rec.Status == "approved" {
		writeJSON(w, http.StatusOK, daemonPairingSessionToResponse(rec, true))
		return
	}

	var req ApproveDaemonPairingSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.WorkspaceID == "" {
		writeError(w, http.StatusBadRequest, "workspace_id is required")
		return
	}

	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireWorkspaceMember(w, r, req.WorkspaceID, "workspace not found"); !ok {
		return
	}

	if h.DB == nil {
		writeError(w, http.StatusInternalServerError, "database executor is not configured")
		return
	}

	if _, err := h.DB.Exec(r.Context(), `
		UPDATE daemon_pairing_session
		SET
			workspace_id = $2,
			approved_by = $3,
			status = 'approved',
			approved_at = now(),
			updated_at = now()
		WHERE token = $1 AND status = 'pending'
	`, token, parseUUID(req.WorkspaceID), parseUUID(userID)); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to approve pairing session")
		return
	}

	rec, err = h.getDaemonPairingSession(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reload pairing session")
		return
	}

	writeJSON(w, http.StatusOK, daemonPairingSessionToResponse(rec, true))
}

func (h *Handler) ClaimDaemonPairingSession(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	rec, err := h.getDaemonPairingSession(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusNotFound, "pairing session not found")
		return
	}
	if rec.Status == "claimed" {
		writeJSON(w, http.StatusOK, daemonPairingSessionToResponse(rec, true))
		return
	}
	if rec.Status != "approved" {
		writeError(w, http.StatusBadRequest, "pairing session is not approved")
		return
	}

	if h.DB == nil {
		writeError(w, http.StatusInternalServerError, "database executor is not configured")
		return
	}

	if _, err := h.DB.Exec(r.Context(), `
		UPDATE daemon_pairing_session
		SET
			status = 'claimed',
			claimed_at = now(),
			updated_at = now()
		WHERE token = $1 AND status = 'approved'
	`, token); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to claim pairing session")
		return
	}

	rec, err = h.getDaemonPairingSession(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reload pairing session")
		return
	}

	resp := daemonPairingSessionToResponse(rec, true)

	// Issue a daemon auth token bound to the workspace and daemon.
	if rec.WorkspaceID.Valid {
		plainToken, err := auth.GenerateDaemonToken()
		if err != nil {
			slog.Error("failed to generate daemon token", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to generate daemon token")
			return
		}
		hash := auth.HashToken(plainToken)

		// Revoke any existing tokens for this workspace+daemon pair.
		_ = h.Queries.DeleteDaemonTokensByWorkspaceAndDaemon(r.Context(), db.DeleteDaemonTokensByWorkspaceAndDaemonParams{
			WorkspaceID: rec.WorkspaceID,
			DaemonID:    rec.DaemonID,
		})

		_, err = h.Queries.CreateDaemonToken(r.Context(), db.CreateDaemonTokenParams{
			TokenHash:   hash,
			WorkspaceID: rec.WorkspaceID,
			DaemonID:    rec.DaemonID,
			ExpiresAt:   pgtype.Timestamptz{Time: time.Now().Add(365 * 24 * time.Hour), Valid: true},
		})
		if err != nil {
			slog.Error("failed to store daemon token", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to store daemon token")
			return
		}

		resp.DaemonToken = &plainToken
		slog.Info("daemon token issued", "daemon_id", rec.DaemonID, "workspace_id", uuidToPtr(rec.WorkspaceID))
	}

	writeJSON(w, http.StatusOK, resp)
}
