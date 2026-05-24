package handler

import (
	"net/http"
)

func (h *Handler) ListCurrentTenantWorkspaces(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	tenantID := requestLarkTenantID(r)
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "Lark tenant session required")
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
	workspaces, err := h.Queries.ListTenantWorkspaces(r.Context(), tenantUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list workspaces")
		return
	}
	resp := make([]WorkspaceResponse, len(workspaces))
	for i, ws := range workspaces {
		resp[i] = workspaceToResponse(ws)
	}
	writeJSON(w, http.StatusOK, resp)
}
