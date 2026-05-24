package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const maxTenantTitleRunes = 120

type TenantSettings struct {
	Title string `json:"title"`
}

type GetTenantSettingsResponse = TenantSettings

type PatchTenantSettingsRequest struct {
	Title *string `json:"title"`
}

type GetTenantRoleResponse struct {
	Role string `json:"role"`
}

func (h *Handler) GetTenantSettings(w http.ResponseWriter, r *http.Request) {
	tenantID := requestLarkTenantID(r)
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "lark tenant id required")
		return
	}
	tenantUUID, ok := parseUUIDOrBadRequest(w, tenantID, "lark tenant id")
	if !ok {
		return
	}

	tenant, err := h.Queries.GetLarkTenant(r.Context(), tenantUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get tenant")
		return
	}

	var settings TenantSettings
	if tenant.Settings != nil {
		json.Unmarshal(tenant.Settings, &settings)
	}

	writeJSON(w, http.StatusOK, settings)
}

func (h *Handler) PatchTenantSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	tenantID := requestLarkTenantID(r)
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "lark tenant id required")
		return
	}
	tenantUUID, ok := parseUUIDOrBadRequest(w, tenantID, "lark tenant id")
	if !ok {
		return
	}

	if _, err := h.getTenantAdmin(r.Context(), userID, tenantUUID); err != nil {
		if !isNotFound(err) {
			writeError(w, http.StatusInternalServerError, "failed to verify tenant admin")
			return
		}
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	var req PatchTenantSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var rawSettings []byte
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if len([]rune(title)) > maxTenantTitleRunes {
			writeError(w, http.StatusBadRequest, "title too long")
			return
		}
		for _, r := range title {
			if unicode.IsControl(r) {
				writeError(w, http.StatusBadRequest, "title contains invalid characters")
				return
			}
		}
		settings := TenantSettings{Title: title}
		rawSettings, _ = json.Marshal(settings)
	} else {
		rawSettings = []byte("{}")
	}

	tenant, err := h.Queries.UpdateLarkTenantSettings(r.Context(), db.UpdateLarkTenantSettingsParams{
		ID:       tenantUUID,
		Settings: rawSettings,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update tenant settings")
		return
	}

	var settings TenantSettings
	if tenant.Settings != nil {
		json.Unmarshal(tenant.Settings, &settings)
	}
	writeJSON(w, http.StatusOK, settings)
}

func (h *Handler) GetTenantRole(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	tenantID := requestLarkTenantID(r)
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "lark tenant id required")
		return
	}
	tenantUUID, ok := parseUUIDOrBadRequest(w, tenantID, "lark tenant id")
	if !ok {
		return
	}

	admin, err := h.getTenantAdmin(r.Context(), userID, tenantUUID)
	if err != nil {
		if isNotFound(err) {
			writeJSON(w, http.StatusOK, GetTenantRoleResponse{Role: "member"})
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to check tenant role")
		return
	}
	writeJSON(w, http.StatusOK, GetTenantRoleResponse{Role: admin.Role})
}
