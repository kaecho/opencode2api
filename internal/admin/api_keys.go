package admin

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"opencode2api/internal/config"
	"opencode2api/internal/httpx"
	"opencode2api/internal/telemetry"
)

// APIKeyView is a server key as the API keys page lists it: its Fingerprint
// ID, its name (a numbered placeholder when none is stored), the masked value,
// whether it is enabled, and its usage.
type APIKeyView struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Display   string      `json:"display"`
	Enabled   bool        `json:"enabled"`
	CreatedAt *time.Time  `json:"created_at,omitempty"`
	Usage     APIKeyUsage `json:"usage"`
}

// APIKeyUsage is in-memory like the rest of telemetry.Metrics: it restarts
// from zero with the process and survives hot reloads. Window covers the last
// hour.
type APIKeyUsage struct {
	Lifetime telemetry.ServerKeyUsage `json:"lifetime"`
	Window   telemetry.ServerKeyUsage `json:"last_hour"`
}

func (a *Server) apiKeyViews(cfg config.Config) []APIKeyView {
	lifetime, window := a.monitor.ServerKeyUsage()
	items := make([]APIKeyView, 0, len(cfg.ServerKeys))
	for i, key := range cfg.ServerKeys {
		id := config.Fingerprint(key)
		meta := cfg.ServerKeyMetadata[id]
		name := meta.Name
		if name == "" {
			name = fmt.Sprintf("Existing key %d", i+1)
		}
		item := APIKeyView{
			ID: id, Name: name, Display: config.MaskValue(key), Enabled: !meta.Disabled,
			Usage: APIKeyUsage{Lifetime: lifetime[id], Window: window[id]},
		}
		if !meta.CreatedAt.IsZero() {
			created := meta.CreatedAt
			item.CreatedAt = &created
		}
		items = append(items, item)
	}
	return items
}

func (a *Server) handleListAPIKeys(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"keys": a.apiKeyViews(a.manager.Config())})
}

// validateKeyName requires a name of 1-80 characters.
func validateKeyName(name string) error {
	if name == "" || len([]rune(name)) > 80 {
		return errors.New("name must contain 1 to 80 characters")
	}
	return nil
}

// addAPIKey appends key as a new server key named name. The name must have
// 1-80 characters and the key 16-256 characters from [A-Za-z0-9._-]; at most
// 200 keys are allowed, and a key whose fingerprint already exists is refused.
func addAPIKey(cfg *config.Config, name, key string) error {
	if err := validateKeyName(name); err != nil {
		return err
	}
	if len(key) < 16 || len(key) > 256 {
		return errors.New("custom key must contain 16 to 256 characters")
	}
	for _, ch := range key {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' || ch == '.') {
			return errors.New("key may contain only letters, digits, hyphen, underscore, and dot")
		}
	}
	if len(cfg.ServerKeys) >= 200 {
		return errors.New("at most 200 API keys may be configured")
	}
	id := config.Fingerprint(key)
	for _, existing := range cfg.ServerKeys {
		if config.Fingerprint(existing) == id {
			return errors.New("this API key already exists")
		}
	}
	if cfg.ServerKeyMetadata == nil {
		cfg.ServerKeyMetadata = map[string]config.ServerKeyMetadata{}
	}
	cfg.ServerKeys = append(cfg.ServerKeys, key)
	cfg.ServerKeyMetadata[id] = config.ServerKeyMetadata{Name: name, CreatedAt: time.Now().UTC()}
	return nil
}

// handleCreateAPIKey adds a server key, generating "sk-local-" plus 48 hex
// digits when the request carries no value, and answers with the plaintext
// key next to the updated, masked list.
func (a *Server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := decodeAdminJSON(w, r, &input); err != nil {
		writeAdminError(w, 400, "invalid_request", err.Error())
		return
	}
	key := input.Value
	if key == "" {
		var random [24]byte
		if _, err := rand.Read(random[:]); err != nil {
			writeAdminError(w, 500, "generation_failed", "could not generate a key")
			return
		}
		key = "sk-local-" + hex.EncodeToString(random[:])
	}
	_, err := a.manager.Update(func(cfg *config.Config) error { return addAPIKey(cfg, strings.TrimSpace(input.Name), key) })
	if err != nil {
		writeAdminError(w, 400, "key_create_failed", a.manager.Redact(err.Error()))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, 201, map[string]any{"key": key, "keys": a.apiKeyViews(a.manager.Config())})
}

// changeAPIKey renames, enables or disables, or (with remove) deletes the
// server key whose fingerprint is id. A change that would leave no enabled
// key is refused.
func changeAPIKey(cfg *config.Config, id string, name *string, enabled *bool, remove bool) error {
	index := -1
	for i, key := range cfg.ServerKeys {
		if config.Fingerprint(key) == id {
			index = i
			break
		}
	}
	if index < 0 {
		return errors.New("API key not found; refresh the list")
	}
	if cfg.ServerKeyMetadata == nil {
		cfg.ServerKeyMetadata = map[string]config.ServerKeyMetadata{}
	}
	if remove {
		cfg.ServerKeys = append(cfg.ServerKeys[:index], cfg.ServerKeys[index+1:]...)
		delete(cfg.ServerKeyMetadata, id)
	} else {
		meta := cfg.ServerKeyMetadata[id]
		if name != nil {
			value := strings.TrimSpace(*name)
			if err := validateKeyName(value); err != nil {
				return err
			}
			meta.Name = value
		}
		if enabled != nil {
			meta.Disabled = !*enabled
		}
		cfg.ServerKeyMetadata[id] = meta
	}
	if cfg.FirstEnabledServerKey() == "" {
		return errors.New("at least one enabled API key must remain; create a replacement first")
	}
	return nil
}

func (a *Server) handleUpdateAPIKey(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name    *string `json:"name"`
		Enabled *bool   `json:"enabled"`
	}
	if err := decodeAdminJSON(w, r, &input); err != nil {
		writeAdminError(w, 400, "invalid_request", err.Error())
		return
	}
	_, err := a.manager.Update(func(cfg *config.Config) error {
		return changeAPIKey(cfg, r.PathValue("id"), input.Name, input.Enabled, false)
	})
	if err != nil {
		writeAdminError(w, 400, "key_update_failed", a.manager.Redact(err.Error()))
		return
	}
	a.handleListAPIKeys(w, r)
}

func (a *Server) handleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	_, err := a.manager.Update(func(cfg *config.Config) error { return changeAPIKey(cfg, r.PathValue("id"), nil, nil, true) })
	if err != nil {
		writeAdminError(w, 400, "key_delete_failed", a.manager.Redact(err.Error()))
		return
	}
	a.handleListAPIKeys(w, r)
}
