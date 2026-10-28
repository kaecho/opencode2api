package admin

import (
	"net/http"
	"strings"

	"opencode2api/internal/config"
	"opencode2api/internal/httpx"
)

// handleResetUpstreamKey clears one upstream key's cooldown and failure
// streak. The key is addressed by its stable fingerprint, never its value.
func (a *Server) handleResetUpstreamKey(w http.ResponseWriter, r *http.Request) {
	tier := config.Tier(strings.TrimSpace(r.PathValue("tier")))
	if tier != config.TierZen && tier != config.TierGo {
		writeAdminError(w, http.StatusBadRequest, "invalid_tier", "tier must be zen or go")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeAdminError(w, http.StatusBadRequest, "invalid_key", "key fingerprint is required")
		return
	}
	if !a.manager.ResetKeyCooldown(tier, id) {
		writeAdminError(w, http.StatusNotFound, "unknown_upstream_key", "upstream key is not configured")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"reset": true, "tier": tier, "id": id})
}
