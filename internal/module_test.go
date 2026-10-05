package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	notifyv1 "github.com/Muxcore-Media/contracts-notification/muxcore/notification/v1"
)

func TestModuleInfo(t *testing.T) {
	m := NewModule(Config{})
	info := m.Info()
	if info.ID == "" {
		t.Error("module ID must not be empty")
	}
	if info.Version == "" {
		t.Error("module version must not be empty")
	}
	if info.MinCoreVersion == "" {
		t.Error("MinCoreVersion must not be empty")
	}
	if len(info.Contracts) == 0 {
		t.Error("Contracts must not be empty")
	}
	if info.HTTPAddr != "127.0.0.1:9445" {
		t.Errorf("default HTTPAddr: got %q", info.HTTPAddr)
	}
}

func TestModuleLifecycle(t *testing.T) {
	m := NewModule(Config{DataDir: t.TempDir(), GRPCAddr: ":0"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := m.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestAppriseNotify(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/notify" {
			t.Errorf("expected /notify, got %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := NewModule(Config{
		GRPCAddr:    "127.0.0.1:0",
		AppriseURL:  srv.URL,
		AppriseURLs: "slack://token_a/token_b/token_c",
	})

	resp, err := m.Notify(context.Background(), &notifyv1.NotifyRequest{
		Title:    "Test Title",
		Message:  "Test body",
		Severity: notifyv1.Severity_SEVERITY_INFO,
	})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if !resp.Results[0].Success {
		t.Fatalf("expected success, got error: %s", resp.Results[0].Error)
	}

	if gotBody["urls"] != "slack://token_a/token_b/token_c" {
		t.Errorf("urls: expected %q, got %v", "slack://token_a/token_b/token_c", gotBody["urls"])
	}
	if gotBody["title"] != "Test Title" {
		t.Errorf("title: expected %q, got %v", "Test Title", gotBody["title"])
	}
	if gotBody["body"] != "Test body" {
		t.Errorf("body: expected %q, got %v", "Test body", gotBody["body"])
	}
	if gotBody["format"] != "text" {
		t.Errorf("format: expected %q, got %v", "text", gotBody["format"])
	}
	if gotBody["priority"] != "low" {
		t.Errorf("priority: expected %q, got %v", "low", gotBody["priority"])
	}
}

func TestAppriseTokenAuth(t *testing.T) {
	var authHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := NewModule(Config{
		GRPCAddr:     "127.0.0.1:0",
		AppriseURL:   srv.URL,
		AppriseURLs:  "slack://token",
		AppriseToken: "my-secret-token",
	})

	_, err := m.Notify(context.Background(), &notifyv1.NotifyRequest{
		Title:   "test",
		Message: "test",
	})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}

	if authHeader != "Bearer my-secret-token" {
		t.Errorf("Authorization header: expected %q, got %q", "Bearer my-secret-token", authHeader)
	}
}

func TestAppriseDefaultURL(t *testing.T) {
	m := NewModule(Config{
		GRPCAddr:    "127.0.0.1:0",
		AppriseURLs: "slack://token",
	})
	if m.appriseURL != "http://localhost:8000" {
		t.Errorf("default Apprise URL: expected %q, got %q", "http://localhost:8000", m.appriseURL)
	}
}

func TestUnconfiguredChannel(t *testing.T) {
	m := NewModule(Config{GRPCAddr: "127.0.0.1:0"})
	resp, err := m.Notify(context.Background(), &notifyv1.NotifyRequest{
		Title:    "test",
		Message:  "test",
		Channels: []notifyv1.Channel{notifyv1.Channel_CHANNEL_APPRISE},
	})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if resp.Results[0].Success {
		t.Error("expected failure for unconfigured channel")
	}
}

func TestNotifyDiscord(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := NewModule(Config{GRPCAddr: ":0", DiscordWebhook: srv.URL})
	ctx := context.Background()

	resp, err := m.Notify(ctx, &notifyv1.NotifyRequest{
		Title:        "Test Discord",
		Message:      "Hello Discord",
		Severity:     notifyv1.Severity_SEVERITY_INFO,
		SourceModule: "test",
		Channels:     []notifyv1.Channel{notifyv1.Channel_CHANNEL_DISCORD},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if !resp.Results[0].Success {
		t.Fatalf("discord webhook failed: %s", resp.Results[0].Error)
	}

	embeds, ok := received["embeds"].([]any)
	if !ok || len(embeds) != 1 {
		t.Fatal("expected discord embed")
	}
	embed := embeds[0].(map[string]any)
	if embed["title"] != "Test Discord" {
		t.Errorf("expected title 'Test Discord', got %v", embed["title"])
	}
}

func TestNotifySlack(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := NewModule(Config{GRPCAddr: ":0", SlackWebhook: srv.URL})

	resp, err := m.Notify(context.Background(), &notifyv1.NotifyRequest{
		Title:    "Test Slack",
		Message:  "Hello Slack",
		Severity: notifyv1.Severity_SEVERITY_WARNING,
		Channels: []notifyv1.Channel{notifyv1.Channel_CHANNEL_SLACK},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Results[0].Success {
		t.Fatalf("slack webhook failed: %s", resp.Results[0].Error)
	}
}

func TestNotifyGenericWebhook(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := NewModule(Config{GRPCAddr: ":0", GenericWebhook: srv.URL})

	resp, err := m.Notify(context.Background(), &notifyv1.NotifyRequest{
		Title:   "Generic",
		Message: "Hello Webhook",
		Fields:  map[string]string{"key": "value"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Results[0].Success {
		t.Fatalf("webhook failed: %s", resp.Results[0].Error)
	}
}

func TestNotifyWebhookFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	m := NewModule(Config{GRPCAddr: ":0", DiscordWebhook: srv.URL})

	resp, err := m.Notify(context.Background(), &notifyv1.NotifyRequest{
		Title:    "Fail",
		Message:  "Should fail",
		Channels: []notifyv1.Channel{notifyv1.Channel_CHANNEL_DISCORD},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Results[0].Success {
		t.Fatal("expected webhook failure")
	}
}

func TestNotifyAllWebhookChannels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := NewModule(Config{
		GRPCAddr:       ":0",
		DiscordWebhook: srv.URL,
		SlackWebhook:   srv.URL,
		GenericWebhook: srv.URL,
	})

	resp, err := m.Notify(context.Background(), &notifyv1.NotifyRequest{
		Title:   "All channels",
		Message: "Test all",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(resp.Results))
	}
	for _, r := range resp.Results {
		if !r.Success {
			t.Errorf("channel %v failed: %s", r.Channel, r.Error)
		}
	}
}

func TestConfigureWebhook(t *testing.T) {
	m := NewModule(Config{DataDir: t.TempDir(), GRPCAddr: ":0"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}

	_, err := m.Configure(ctx, &notifyv1.ConfigureRequest{
		Channel:  notifyv1.Channel_CHANNEL_DISCORD,
		Settings: map[string]string{"webhook_url": "https://discord.example.com/new"},
	})
	if err != nil {
		t.Fatal(err)
	}

	m.mu.RLock()
	cfg, ok := m.channels[notifyv1.Channel_CHANNEL_DISCORD]
	m.mu.RUnlock()
	if !ok {
		t.Fatal("channel not found after Configure")
	}
	if !cfg.Enabled {
		t.Error("channel should be enabled")
	}
	if cfg.Webhook != "https://discord.example.com/new" {
		t.Errorf("webhook: expected %q, got %q", "https://discord.example.com/new", cfg.Webhook)
	}
}

func TestConfigureRejectsEmail(t *testing.T) {
	m := NewModule(Config{DataDir: t.TempDir(), GRPCAddr: ":0"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	_, err := m.Configure(ctx, &notifyv1.ConfigureRequest{
		Channel: notifyv1.Channel_CHANNEL_EMAIL,
	})
	if err == nil {
		t.Fatal("expected error for email channel")
	}
}

func TestConfigureRejectsInsecureWebhook(t *testing.T) {
	m := NewModule(Config{DataDir: t.TempDir(), GRPCAddr: ":0"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	_, err := m.Configure(ctx, &notifyv1.ConfigureRequest{
		Channel:  notifyv1.Channel_CHANNEL_DISCORD,
		Settings: map[string]string{"webhook_url": "http://127.0.0.1/hook"},
	})
	if err == nil {
		t.Fatal("expected error for non-https webhook")
	}
}

func TestBuildPayloads(t *testing.T) {
	req := &notifyv1.NotifyRequest{
		Title:        "Test",
		Message:      "Message",
		Severity:     notifyv1.Severity_SEVERITY_ERROR,
		SourceModule: "test-module",
		Fields:       map[string]string{"Key": "Value"},
	}

	t.Run("discord", func(t *testing.T) {
		b, err := buildDiscordPayload(req)
		if err != nil {
			t.Fatal(err)
		}
		var p map[string]any
		if err := json.Unmarshal(b, &p); err != nil {
			t.Fatal(err)
		}
		embeds := p["embeds"].([]any)
		embed := embeds[0].(map[string]any)
		if embed["title"] != "Test" {
			t.Errorf("title: expected %q, got %v", "Test", embed["title"])
		}
		if color := embed["color"]; color != float64(0xE74C3C) {
			t.Errorf("color: expected %d, got %v", 0xE74C3C, color)
		}
	})

	t.Run("slack", func(t *testing.T) {
		b, err := buildSlackPayload(req)
		if err != nil {
			t.Fatal(err)
		}
		var p map[string]any
		if err := json.Unmarshal(b, &p); err != nil {
			t.Fatal(err)
		}
		atts := p["attachments"].([]any)
		att := atts[0].(map[string]any)
		if att["color"] != "danger" {
			t.Errorf("color: expected 'danger', got %v", att["color"])
		}
	})

	t.Run("webhook", func(t *testing.T) {
		b, err := buildWebhookPayload(req)
		if err != nil {
			t.Fatal(err)
		}
		var p map[string]any
		if err := json.Unmarshal(b, &p); err != nil {
			t.Fatal(err)
		}
		if p["title"] != "Test" {
			t.Errorf("title: expected 'Test', got %v", p["title"])
		}
		if p["severity"] != "ERROR" {
			t.Errorf("severity: expected 'ERROR', got %v", p["severity"])
		}
	})
}

func TestColorHelpers(t *testing.T) {
	tests := []struct {
		severity notifyv1.Severity
		discord  int
		slack    string
	}{
		{notifyv1.Severity_SEVERITY_ERROR, 0xE74C3C, "danger"},
		{notifyv1.Severity_SEVERITY_WARNING, 0xF39C12, "warning"},
		{notifyv1.Severity_SEVERITY_SUCCESS, 0x2ECC71, "good"},
		{notifyv1.Severity_SEVERITY_INFO, 0x3498DB, "good"},
		{notifyv1.Severity_SEVERITY_UNSPECIFIED, 0x3498DB, "good"},
	}

	for _, tt := range tests {
		if got := discordColor(tt.severity); got != tt.discord {
			t.Errorf("discordColor(%v) = %d, want %d", tt.severity, got, tt.discord)
		}
		if got := slackColor(tt.severity); got != tt.slack {
			t.Errorf("slackColor(%v) = %q, want %q", tt.severity, got, tt.slack)
		}
	}
}

func TestHealthWithWebhookChannel(t *testing.T) {
	m := NewModule(Config{GRPCAddr: "127.0.0.1:0", DiscordWebhook: "https://discord.gg/webhook"})
	err := m.Health(context.Background())
	if err != nil {
		t.Errorf("expected Health() to pass, got: %v", err)
	}
}

func TestStatusMultipleChannels(t *testing.T) {
	m := NewModule(Config{
		GRPCAddr:       "127.0.0.1:0",
		AppriseURLs:    "slack://token",
		DiscordWebhook: "https://discord.gg/webhook",
	})
	resp, err := m.Status(context.Background(), &notifyv1.StatusRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Channels) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(resp.Channels))
	}
}

func TestStatusMasksSecrets(t *testing.T) {
	secretURL := "https://discord.com/api/webhooks/123456789/abcdefghijklmnopqrstuvwxyz"
	appriseURL := "slack://token_a/token_b/token_c"
	m := NewModule(Config{
		GRPCAddr:       "127.0.0.1:0",
		AppriseURLs:    appriseURL,
		DiscordWebhook: secretURL,
	})
	resp, err := m.Status(context.Background(), &notifyv1.StatusRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range resp.Channels {
		if strings.Contains(st.Description, secretURL) {
			t.Fatalf("status leaked webhook: %q", st.Description)
		}
		if strings.Contains(st.Description, appriseURL) {
			t.Fatalf("status leaked apprise URL: %q", st.Description)
		}
		if strings.Contains(st.Description, "token_a") {
			t.Fatalf("status leaked slack token: %q", st.Description)
		}
	}
}

func TestHealthNoChannels(t *testing.T) {
	m := NewModule(Config{GRPCAddr: "127.0.0.1:0"})
	err := m.Health(context.Background())
	if err == nil {
		t.Error("expected Health() to fail with no channels configured")
	}
}

func TestHealthWithChannel(t *testing.T) {
	m := NewModule(Config{GRPCAddr: "127.0.0.1:0", AppriseURLs: "slack://token"})
	err := m.Health(context.Background())
	if err != nil {
		t.Errorf("expected Health() to pass, got: %v", err)
	}
}

func TestConfigure(t *testing.T) {
	m := NewModule(Config{GRPCAddr: "127.0.0.1:0", DataDir: t.TempDir()})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	_, err := m.Configure(ctx, &notifyv1.ConfigureRequest{
		Channel: notifyv1.Channel_CHANNEL_APPRISE,
		Settings: map[string]string{
			"urls": "tgram://bot_token/chat_id",
		},
	})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}

	m.mu.RLock()
	cfg, ok := m.channels[notifyv1.Channel_CHANNEL_APPRISE]
	m.mu.RUnlock()
	if !ok {
		t.Fatal("channel not found after Configure")
	}
	if !cfg.Enabled {
		t.Error("channel should be enabled")
	}
	if cfg.URLs != "tgram://bot_token/chat_id" {
		t.Errorf("urls: expected %q, got %q", "tgram://bot_token/chat_id", cfg.URLs)
	}
}

func TestStatus(t *testing.T) {
	m := NewModule(Config{GRPCAddr: "127.0.0.1:0", AppriseURLs: "slack://token"})
	resp, err := m.Status(context.Background(), &notifyv1.StatusRequest{})
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(resp.Channels) == 0 {
		t.Fatal("expected at least one channel in status")
	}
	ch := resp.Channels[0]
	if ch.Channel != notifyv1.Channel_CHANNEL_APPRISE {
		t.Errorf("channel: expected %v, got %v", notifyv1.Channel_CHANNEL_APPRISE, ch.Channel)
	}
	if !ch.Enabled {
		t.Error("channel should be enabled")
	}
}
