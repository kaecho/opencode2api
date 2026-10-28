package admin

import (
	"testing"

	"opencode2api/internal/config"
)

func TestAddRemoveUpstreamKey(t *testing.T) {
	cfg := config.Config{ZenKeys: []string{"zen-key-1234567890"}}
	if err := addUpstreamKey(&cfg, config.TierZen, "zen-key-0987654321"); err != nil {
		t.Fatal(err)
	}
	if len(cfg.ZenKeys) != 2 {
		t.Fatalf("zen keys = %d, want 2", len(cfg.ZenKeys))
	}
	if err := addUpstreamKey(&cfg, config.TierZen, "zen-key-0987654321"); err == nil {
		t.Fatal("duplicate key was accepted")
	}
	if err := removeUpstreamKey(&cfg, config.TierZen, config.Fingerprint("zen-key-0987654321")); err != nil {
		t.Fatal(err)
	}
	if len(cfg.ZenKeys) != 1 || cfg.ZenKeys[0] != "zen-key-1234567890" {
		t.Fatalf("unexpected zen keys: %v", cfg.ZenKeys)
	}
	if err := removeUpstreamKey(&cfg, config.TierZen, "missing"); err == nil {
		t.Fatal("missing key was removed")
	}
	if err := addUpstreamKey(&cfg, config.TierGo, "go-key-1234567890"); err != nil {
		t.Fatal(err)
	}
	if len(cfg.GoKeys) != 1 {
		t.Fatalf("go keys = %d, want 1", len(cfg.GoKeys))
	}
	if err := addUpstreamKey(&cfg, config.TierZen, "short"); err == nil {
		t.Fatal("short key was accepted")
	}
}
