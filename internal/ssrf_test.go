package internal

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	notifyv1 "github.com/Muxcore-Media/contracts-notification/muxcore/notification/v1"
)

func TestValidateWebhookURLBlocked(t *testing.T) {
	for _, u := range []string{
		"http://hooks.slack.com/x", "https://127.0.0.1/x", "https://localhost/x",
		"https://10.0.0.1/x", "https://172.16.5.5/x", "https://192.168.0.1/x",
		"https://169.254.169.254/latest", "https://[::1]/x", "https://[fe80::1]/x",
		"https://0x7f000001/x", "https://private.test/x", "https://metadata.google.internal/x",
		"https://100.64.0.1/x", "ftp://example.com/x", "https://user:p@example.com/x",
	} {
		if err := validateWebhookURL(context.Background(), u); err == nil {
			t.Errorf("accepted %q", u)
		}
	}
	if err := validateWebhookURL(context.Background(), "https://hooks.slack.com/services/x"); err != nil {
		t.Errorf("public rejected: %v", err)
	}
}

func TestValidateAppriseURL(t *testing.T) {
	for _, u := range []string{"http://localhost:8000", "http://127.0.0.1:8000", "http://192.168.1.20:8000", "http://apprise:8000", "https://apprise.lan"} {
		if err := validateAppriseURL(u); err != nil {
			t.Errorf("LAN apprise %q rejected: %v", u, err)
		}
	}
	for _, u := range []string{"http://169.254.169.254/", "http://metadata.google.internal/", "file:///etc/passwd", "gopher://x", "http://[fe80::1]:8000"} {
		if err := validateAppriseURL(u); err == nil {
			t.Errorf("apprise %q accepted", u)
		}
	}
}

func TestUpdateSettingRejectsBadAppriseURL(t *testing.T) {
	m := NewModule(Config{DataDir: t.TempDir(), GRPCAddr: ":0"})
	if err := m.UpdateSetting("apprise_url", "http://169.254.169.254"); err == nil {
		t.Fatal("metadata apprise_url accepted")
	}
	if m.appriseURL != "http://localhost:8000" {
		t.Fatalf("appriseURL changed: %q", m.appriseURL)
	}
	if err := m.UpdateSetting("apprise_url", "http://192.168.1.5:8000/"); err != nil {
		t.Fatalf("LAN apprise rejected: %v", err)
	}
}

func TestSendBlockedAtDialTime(t *testing.T) {
	for _, u := range []string{"https://127.0.0.1:1/x", "https://10.1.2.3/x", "http://example.com/x"} {
		m := NewModule(Config{GRPCAddr: ":0", GenericWebhook: u})
		res := m.sendWebhook(context.Background(), notifyv1.Channel_CHANNEL_WEBHOOK,
			&channelConfig{Webhook: u}, &notifyv1.NotifyRequest{Title: "t", Message: "m"})
		if res.GetSuccess() || !strings.Contains(res.GetError(), "blocked") {
			t.Errorf("send to %q: %+v", u, res)
		}
	}
}

func TestSendAppriseBlocksMetadata(t *testing.T) {
	m := NewModule(Config{GRPCAddr: ":0", AppriseURL: "http://169.254.169.254", AppriseURLs: "slack://a/b/c"})
	res := m.sendApprise(context.Background(), &channelConfig{URLs: "slack://a/b/c"}, &notifyv1.NotifyRequest{Title: "t", Message: "m"})
	if res.GetSuccess() {
		t.Fatal("apprise to metadata IP succeeded")
	}
}

func TestPersistedUnsafeValuesIgnored(t *testing.T) {
	dir := t.TempDir()
	ps := persistedSettings{
		AppriseURL: "http://169.254.169.254",
		Channels: map[string]persistedChannel{
			"CHANNEL_WEBHOOK": {Enabled: true, Webhook: "https://10.0.0.1/x", Settings: map[string]string{"type": "generic"}},
			"CHANNEL_DISCORD": {Enabled: true, Webhook: "https://discord.com/api/webhooks/1/x", Settings: map[string]string{"type": "discord"}},
		},
	}
	b, _ := json.Marshal(ps)
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	m := NewModule(Config{DataDir: dir, GRPCAddr: ":0"})
	if err := m.loadPersisted(); err != nil {
		t.Fatal(err)
	}
	if m.appriseURL != "http://localhost:8000" {
		t.Errorf("persisted metadata apprise_url loaded: %q", m.appriseURL)
	}
	if _, ok := m.channels[notifyv1.Channel_CHANNEL_WEBHOOK]; ok {
		t.Error("persisted private webhook loaded")
	}
	if _, ok := m.channels[notifyv1.Channel_CHANNEL_DISCORD]; !ok {
		t.Error("persisted public discord webhook dropped")
	}
}
