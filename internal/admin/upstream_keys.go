package admin

import (
	"errors"
	"net/http"
	"strings"
	"unicode"

	"opencode2api/internal/config"
	"opencode2api/internal/httpx"
)

func upstreamKeyList(cfg *config.Config, tier config.Tier) (*[]string, bool) {
	switch tier {
	case config.TierZen:
		return &cfg.ZenKeys, true
	case config.TierGo:
		return &cfg.GoKeys, true
	default:
		return nil, false
	}
}

// addUpstreamKey appends key to the selected tier. Keys are stored masked in
// the configuration; the fingerprint is the only identifier returned to the
// browser.
func addUpstreamKey(cfg *config.Config, tier config.Tier, key string) error {
	key = strings.TrimSpace(key)
	if len(key) < 16 || len(key) > 512 {
		return errors.New("key must contain 16 to 512 characters")
	}
	if strings.IndexFunc(key, unicode.IsSpace) >= 0 {
		return errors.New("key must not contain whitespace")
	}
	keys, ok := upstreamKeyList(cfg, tier)
	if !ok {
		return errors.New("tier must be zen or go")
	}
	id := config.Fingerprint(key)
	for _, existing := range *keys {
		if config.Fingerprint(existing) == id {
			return errors.New("upstream key already exists")
		}
	}
	*keys = append(*keys, key)
	return nil
}

// removeUpstreamKey removes the key with fingerprint id from the selected
// tier. Normalize still decides whether the remaining configuration has any
// usable upstream.
func removeUpstreamKey(cfg *config.Config, tier config.Tier, id string) error {
	keys, ok := upstreamKeyList(cfg, tier)
	if !ok {
		return errors.New("tier must be zen or go")
	}
	for i, existing := range *keys {
		if config.Fingerprint(existing) == id {
			*keys = append((*keys)[:i], (*keys)[i+1:]...)
			return nil
		}
	}
	return errors.New("upstream key not found")
}

// handleCreateUpstreamKey adds a Zen or Go upstream key. The plaintext key is
// never echoed back; the response carries only its stable fingerprint.
func (a *Server) handleCreateUpstreamKey(w http.ResponseWriter, r *http.Request) {
	tier := config.Tier(strings.TrimSpace(r.PathValue("tier")))
	var input struct {
		Key string `json:"key"`
	}
	if err := decodeAdminJSON(w, r, &input); err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if _, ok := upstreamKeyList(&config.Config{}, tier); !ok {
		writeAdminError(w, http.StatusBadRequest, "invalid_tier", "tier must be zen or go")
		return
	}
	if _, err := a.manager.Update(func(cfg *config.Config) error { return addUpstreamKey(cfg, tier, input.Key) }); err != nil {
		writeAdminError(w, http.StatusBadRequest, "upstream_key_create_failed", a.manager.Redact(err.Error()))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"created": true, "tier": tier, "id": config.Fingerprint(strings.TrimSpace(input.Key))})
}

// handleDeleteUpstreamKey removes one Zen or Go upstream key by fingerprint.
func (a *Server) handleDeleteUpstreamKey(w http.ResponseWriter, r *http.Request) {
	tier := config.Tier(strings.TrimSpace(r.PathValue("tier")))
	id := strings.TrimSpace(r.PathValue("id"))
	if _, ok := upstreamKeyList(&config.Config{}, tier); !ok {
		writeAdminError(w, http.StatusBadRequest, "invalid_tier", "tier must be zen or go")
		return
	}
	if id == "" {
		writeAdminError(w, http.StatusBadRequest, "invalid_key", "key fingerprint is required")
		return
	}
	if _, err := a.manager.Update(func(cfg *config.Config) error { return removeUpstreamKey(cfg, tier, id) }); err != nil {
		writeAdminError(w, http.StatusBadRequest, "upstream_key_delete_failed", a.manager.Redact(err.Error()))
		return
	}
	a.manager.DeleteKeyState(tier, id)
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "tier": tier, "id": id})
}

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
