package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	notifyv1 "github.com/Muxcore-Media/contracts-notification/muxcore/notification/v1"
)

func TestSettingsAppriseAndWebhooks(t *testing.T) {
	dir := t.TempDir()
	m := NewModule(Config{DataDir: dir, AppriseURL: "http://127.0.0.1:8000", AppriseURLs: "mailto://old"})
	if err := m.Init(t.Context()); err != nil {
		t.Fatal(err)
	}
	defs := m.Settings()
	if len(defs) < 15 {
		t.Fatalf("defs=%d", len(defs))
	}
	if err := m.UpdateSetting("apprise_url", "http://127.0.0.1:9000/"); err != nil {
		t.Fatal(err)
	}
	base, _ := m.getAppriseEndpoint()
	if base != "http://127.0.0.1:9000" {
		t.Fatalf("base=%q", base)
	}
	if err := m.UpdateSetting("discord_webhook", "https://discord.example.com/hook"); err != nil {
		t.Fatal(err)
	}
	m.mu.RLock()
	cfg := m.channels[notifyv1.Channel_CHANNEL_DISCORD]
	m.mu.RUnlock()
	if cfg == nil || cfg.Webhook != "https://discord.example.com/hook" {
		t.Fatalf("discord=%+v", cfg)
	}
	if err := m.UpdateSetting("discord_webhook", "********"); err != nil {
		t.Fatal(err)
	}
	m.mu.RLock()
	cfg = m.channels[notifyv1.Channel_CHANNEL_DISCORD]
	m.mu.RUnlock()
	if cfg.Webhook != "https://discord.example.com/hook" {
		t.Fatalf("mask overwrite: %q", cfg.Webhook)
	}
}

func TestSettingsPersistAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	m := NewModule(Config{DataDir: dir, GRPCAddr: ":0"})
	if err := m.Init(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateSetting("discord_webhook", "https://discord.example.com/hook"); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateSetting("notify_requested", "false"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "https://discord.example.com/hook") {
		t.Fatalf("settings not persisted: %s", data)
	}

	m2 := NewModule(Config{DataDir: dir, GRPCAddr: ":0"})
	if err := m2.Init(t.Context()); err != nil {
		t.Fatal(err)
	}
	m2.mu.RLock()
	cfg := m2.channels[notifyv1.Channel_CHANNEL_DISCORD]
	prefs := m2.prefs
	m2.mu.RUnlock()
	if cfg == nil || cfg.Webhook != "https://discord.example.com/hook" {
		t.Fatalf("discord not reloaded: %+v", cfg)
	}
	if prefs.NotifyRequested {
		t.Fatal("expected notify_requested=false after reload")
	}
}

func TestEventPrefMute(t *testing.T) {
	m := NewModule(Config{})
	m.prefs.NotifyRequested = false
	if m.prefs.allows("media.movie.requested") {
		t.Fatal("expected requested events muted")
	}
	if !m.prefs.allows("download.failed") {
		t.Fatal("expected download.failed allowed by default")
	}
}

func TestUpdateSettingRejectsInsecureWebhook(t *testing.T) {
	m := NewModule(Config{DataDir: t.TempDir()})
	if err := m.Init(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateSetting("webhook_url", "http://127.0.0.1/hook"); err == nil {
		t.Fatal("expected error for non-https webhook")
	}
}
