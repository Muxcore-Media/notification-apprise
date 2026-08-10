package internal

import (
	"testing"

	notifyv1 "github.com/Muxcore-Media/contracts-notification/muxcore/notification/v1"
)

func TestSettingsAppriseAndWebhooks(t *testing.T) {
	m := NewModule(Config{AppriseURL: "http://127.0.0.1:8000", AppriseURLs: "mailto://old"})
	defs := m.Settings()
	if len(defs) < 6 {
		t.Fatalf("defs=%d", len(defs))
	}
	if err := m.UpdateSetting("apprise_url", "http://127.0.0.1:9000/"); err != nil {
		t.Fatal(err)
	}
	base, _ := m.getAppriseEndpoint()
	if base != "http://127.0.0.1:9000" {
		t.Fatalf("base=%q", base)
	}
	if err := m.UpdateSetting("discord_webhook", "http://127.0.0.1:9/hook"); err != nil {
		t.Fatal(err)
	}
	m.mu.RLock()
	cfg := m.channels[notifyv1.Channel_CHANNEL_DISCORD]
	m.mu.RUnlock()
	if cfg == nil || cfg.Webhook != "http://127.0.0.1:9/hook" {
		t.Fatalf("discord=%+v", cfg)
	}
	if err := m.UpdateSetting("discord_webhook", "********"); err != nil {
		t.Fatal(err)
	}
	m.mu.RLock()
	cfg = m.channels[notifyv1.Channel_CHANNEL_DISCORD]
	m.mu.RUnlock()
	if cfg.Webhook != "http://127.0.0.1:9/hook" {
		t.Fatalf("mask overwrite: %q", cfg.Webhook)
	}
}
