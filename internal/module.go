package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"

	notifyv1 "github.com/Muxcore-Media/contracts-notification/muxcore/notification/v1"
	"github.com/Muxcore-Media/core/pkg/contracts"
	eventsv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/events/v1"
	"github.com/Muxcore-Media/core/sdk/go/client"
)

type channelConfig struct {
	Enabled  bool
	URLs     string
	Webhook  string
	Settings map[string]string
}

type Module struct {
	notifyv1.UnimplementedNotificationServiceServer

	mu       sync.RWMutex
	channels map[notifyv1.Channel]*channelConfig
	client   *http.Client
	mc       *client.Client

	id           string
	grpcAddr     string
	appriseURL   string
	appriseURLs  string
	appriseToken string
	grpcSrv      *grpc.Server
	lis          net.Listener
}

type Config struct {
	ID             string
	GRPCAddr       string
	AppriseURL     string
	AppriseURLs    string
	AppriseToken   string
	DiscordWebhook string
	SlackWebhook   string
	GenericWebhook string
}

func NewModule(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "notification-apprise"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = ":9445"
	}
	if cfg.AppriseURL == "" {
		cfg.AppriseURL = "http://localhost:8000"
	}
	if v := os.Getenv("NOTIFY_GRPC_ADDR"); v != "" {
		cfg.GRPCAddr = v
	}
	if v := os.Getenv("APPRISE_URL"); v != "" {
		cfg.AppriseURL = v
	}
	if v := os.Getenv("APPRISE_URLS"); v != "" {
		cfg.AppriseURLs = v
	}
	if v := os.Getenv("APPRISE_TOKEN"); v != "" {
		cfg.AppriseToken = v
	}
	if v := os.Getenv("DISCORD_WEBHOOK"); v != "" {
		cfg.DiscordWebhook = v
	}
	if v := os.Getenv("SLACK_WEBHOOK"); v != "" {
		cfg.SlackWebhook = v
	}
	if v := os.Getenv("WEBHOOK_URL"); v != "" {
		cfg.GenericWebhook = v
	}

	m := &Module{
		id:           cfg.ID,
		grpcAddr:     cfg.GRPCAddr,
		appriseURL:   cfg.AppriseURL,
		appriseURLs:  cfg.AppriseURLs,
		appriseToken: cfg.AppriseToken,
		client:       &http.Client{Timeout: 10 * time.Second},
		channels:     map[notifyv1.Channel]*channelConfig{},
	}

	if cfg.AppriseURLs != "" {
		m.channels[notifyv1.Channel_CHANNEL_APPRISE] = &channelConfig{
			Enabled: true,
			URLs:    cfg.AppriseURLs,
			Settings: map[string]string{
				"type": "apprise",
				"urls": cfg.AppriseURLs,
			},
		}
	}
	if cfg.DiscordWebhook != "" {
		m.channels[notifyv1.Channel_CHANNEL_DISCORD] = &channelConfig{
			Enabled: true, Webhook: cfg.DiscordWebhook,
			Settings: map[string]string{"type": "discord"},
		}
	}
	if cfg.SlackWebhook != "" {
		m.channels[notifyv1.Channel_CHANNEL_SLACK] = &channelConfig{
			Enabled: true, Webhook: cfg.SlackWebhook,
			Settings: map[string]string{"type": "slack"},
		}
	}
	if cfg.GenericWebhook != "" {
		m.channels[notifyv1.Channel_CHANNEL_WEBHOOK] = &channelConfig{
			Enabled: true, Webhook: cfg.GenericWebhook,
			Settings: map[string]string{"type": "generic"},
		}
	}

	return m
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID:           m.id,
		Name:         "Notification Apprise",
		Version:      "0.1.3",
		Roles:        []string{"notification"},
		Description:  "Apprise multi-platform notifications with Discord, Slack, and webhook channel support",
		Author:       "MuxCore",
		Capabilities: []string{"notification", "notification.apprise"},
		Contracts: []contracts.ContractDeclaration{
			{
				Repo:      "github.com/Muxcore-Media/contracts-notification",
				Interface: "NotificationProvider",
				Version:   "v0.1.0",
			},
		},
		MinCoreVersion: "0.4.0",
		HTTPAddr:       m.grpcAddr,
	}
}

func (m *Module) Init(ctx context.Context) error {
	lis, err := net.Listen("tcp", m.grpcAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", m.grpcAddr, err)
	}
	m.lis = lis
	slog.Info("notification-apprise initialized", "addr", m.grpcAddr)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	m.grpcSrv = grpc.NewServer()
	notifyv1.RegisterNotificationServiceServer(m.grpcSrv, m)
	go func() {
		slog.Info("notification-apprise gRPC service started", "addr", m.grpcAddr)
		if err := m.grpcSrv.Serve(m.lis); err != nil {
			slog.Error("notification-apprise gRPC serve error", "error", err)
		}
	}()
	go m.dialCore(context.Background())
	go m.subscribeToMediaEvents()
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	if m.grpcSrv != nil {
		m.grpcSrv.GracefulStop()
	}
	m.client.CloseIdleConnections()
	slog.Info("notification-apprise stopped")
	return nil
}

func (m *Module) Health(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, cfg := range m.channels {
		if cfg.Enabled {
			return nil
		}
	}
	return fmt.Errorf("no notification channels configured — set APPRISE_URLS, DISCORD_WEBHOOK, SLACK_WEBHOOK, or WEBHOOK_URL")
}

func (m *Module) dialCore(ctx context.Context) {
	meshAddr := os.Getenv("MUXCORE_GRPC_ADDR")
	if meshAddr == "" {
		meshAddr = "localhost:9090"
	}
	insecureMode := os.Getenv("MUXCORE_INSECURE_DISABLE_TLS") == "true" || os.Getenv("MUXCORE_GRPC_INSECURE") == "true"
	var opts []client.Option
	if insecureMode {
		opts = append(opts, client.WithInsecure())
	}
	c, err := client.Dial(meshAddr, opts...)
	if err != nil {
		slog.Error("notification-apprise: dial core", "error", err)
		return
	}
	m.mc = c
	slog.Info("notification-apprise: connected to core mesh", "addr", meshAddr)
}

func (m *Module) subscribeToMediaEvents() {
	time.Sleep(15 * time.Second)
	if m.mc == nil {
		slog.Warn("notification-apprise: not connected to core, skipping event subscriptions")
		return
	}

	eventTypes := []string{
		contracts.EventMovieAdded,
		contracts.EventMovieRemoved,
		contracts.EventMovieFileAdded,
		contracts.EventTVAdded,
		contracts.EventTVRemoved,
		contracts.EventTVEpisodeFileAdded,
		contracts.EventFileImported,
		contracts.EventDownloadDispatched,
	}

	for _, et := range eventTypes {
		ch, cancel, err := m.mc.Events.Subscribe(context.Background(), et)
		if err != nil {
			slog.Warn("subscribe to event", "type", et, "error", err)
			continue
		}
		go m.handleEventStream(et, ch, cancel)
		slog.Info("subscribed to events", "type", et)
	}
}

func (m *Module) handleEventStream(eventType string, ch <-chan *eventsv1.Event, cancel context.CancelFunc) {
	for evt := range ch {
		title, message, severity, fields := m.formatNotification(eventType, evt.Payload)
		if title == "" {
			continue
		}
		m.Notify(context.Background(), &notifyv1.NotifyRequest{
			Title:        title,
			Message:      message,
			Severity:     severity,
			SourceModule: evt.Source,
			Fields:       fields,
		})
	}
	cancel()
}

func (m *Module) formatNotification(eventType string, payload []byte) (title, message, severity string, fields map[string]string) {
	fields = map[string]string{}

	switch eventType {
	case contracts.EventMovieAdded:
		var p contracts.MovieAddedPayload
		if json.Unmarshal(payload, &p) != nil {
			return
		}
		return "Movie Added", p.Title, "success", map[string]string{
			"TMDB ID": strconv.Itoa(int(p.TMDBID)), "Movie ID": p.MovieID,
		}

	case contracts.EventMovieRemoved:
		var p contracts.MovieRemovedPayload
		if json.Unmarshal(payload, &p) != nil {
			return
		}
		return "Movie Removed", p.Title, "warning", map[string]string{"TMDB ID": strconv.Itoa(int(p.TMDBID))}

	case contracts.EventMovieFileAdded:
		var p contracts.MovieFileAddedPayload
		if json.Unmarshal(payload, &p) != nil {
			return
		}
		return "Movie File Added", p.FilePath, "info", map[string]string{
			"Quality": p.Quality, "Movie ID": p.MovieID,
		}

	case contracts.EventTVAdded:
		var p contracts.TVAddedPayload
		if json.Unmarshal(payload, &p) != nil {
			return
		}
		return "TV Show Added", p.Name, "success", map[string]string{
			"TMDB ID": strconv.Itoa(int(p.TMDBID)), "Series ID": p.SeriesID,
		}

	case contracts.EventTVRemoved:
		var p contracts.TVRemovedPayload
		if json.Unmarshal(payload, &p) != nil {
			return
		}
		return "TV Show Removed", p.SeriesID, "warning", nil

	case contracts.EventTVEpisodeFileAdded:
		var p contracts.TVEpisodeFileAddedPayload
		if json.Unmarshal(payload, &p) != nil {
			return
		}
		return "Episode File Added", p.FilePath, "info", map[string]string{
			"Quality": p.Quality, "Episode ID": p.EpisodeID,
		}

	case contracts.EventFileImported:
		var p contracts.FileImportedPayload
		if json.Unmarshal(payload, &p) != nil {
			return
		}
		label := "File Imported"
		if p.MediaType == "tv" {
			label = "Episode Imported"
		}
		return label, fmt.Sprintf("%s (%d)", p.Title, p.Year), "success", map[string]string{
			"Type": p.MediaType, "Quality": p.Quality, "Path": p.DestinationPath,
		}

	case contracts.EventDownloadDispatched:
		var p contracts.DownloadDispatchedPayload
		if json.Unmarshal(payload, &p) != nil {
			return
		}
		return "Download Dispatched", p.Title, "info", map[string]string{
			"Protocol": p.DownloadProtocol, "Score": strconv.Itoa(int(p.Score)),
		}
	}

	return
}

func (m *Module) Notify(ctx context.Context, req *notifyv1.NotifyRequest) (*notifyv1.NotifyResponse, error) {
	channels := req.GetChannels()
	if len(channels) == 0 {
		m.mu.RLock()
		for ch := range m.channels {
			channels = append(channels, ch)
		}
		m.mu.RUnlock()
	}

	var results []*notifyv1.ChannelResult
	for _, ch := range channels {
		r := m.sendToChannel(ctx, ch, req)
		results = append(results, r)
	}

	return &notifyv1.NotifyResponse{Results: results}, nil
}

func (m *Module) sendToChannel(ctx context.Context, ch notifyv1.Channel, req *notifyv1.NotifyRequest) *notifyv1.ChannelResult {
	m.mu.RLock()
	cfg, ok := m.channels[ch]
	m.mu.RUnlock()

	if !ok || !cfg.Enabled {
		return &notifyv1.ChannelResult{
			Channel: ch, Success: false,
			Error: fmt.Sprintf("channel %s not configured", ch),
		}
	}

	switch ch {
	case notifyv1.Channel_CHANNEL_APPRISE:
		return m.sendApprise(ctx, cfg, req)
	case notifyv1.Channel_CHANNEL_DISCORD:
		return m.sendWebhook(ctx, ch, cfg, req)
	case notifyv1.Channel_CHANNEL_SLACK:
		return m.sendWebhook(ctx, ch, cfg, req)
	case notifyv1.Channel_CHANNEL_WEBHOOK:
		return m.sendWebhook(ctx, ch, cfg, req)
	default:
		return &notifyv1.ChannelResult{Channel: ch, Success: false, Error: "unsupported channel"}
	}
}

func (m *Module) sendApprise(ctx context.Context, cfg *channelConfig, req *notifyv1.NotifyRequest) *notifyv1.ChannelResult {
	payload, err := m.buildApprisePayload(req, cfg.URLs)
	if err != nil {
		return &notifyv1.ChannelResult{
			Channel: notifyv1.Channel_CHANNEL_APPRISE,
			Success: false,
			Error:   err.Error(),
		}
	}

	url := m.appriseURL + "/notify"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return &notifyv1.ChannelResult{
			Channel: notifyv1.Channel_CHANNEL_APPRISE,
			Success: false,
			Error:   err.Error(),
		}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if m.appriseToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+m.appriseToken)
	}

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return &notifyv1.ChannelResult{
			Channel: notifyv1.Channel_CHANNEL_APPRISE,
			Success: false,
			Error:   err.Error(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return &notifyv1.ChannelResult{
			Channel: notifyv1.Channel_CHANNEL_APPRISE,
			Success: false,
			Error:   fmt.Sprintf("apprise returned %s", resp.Status),
		}
	}

	return &notifyv1.ChannelResult{
		Channel: notifyv1.Channel_CHANNEL_APPRISE,
		Success: true,
	}
}

func (m *Module) sendWebhook(ctx context.Context, ch notifyv1.Channel, cfg *channelConfig, req *notifyv1.NotifyRequest) *notifyv1.ChannelResult {
	var payload []byte
	var err error

	switch ch {
	case notifyv1.Channel_CHANNEL_DISCORD:
		payload, err = buildDiscordPayload(req)
	case notifyv1.Channel_CHANNEL_SLACK:
		payload, err = buildSlackPayload(req)
	case notifyv1.Channel_CHANNEL_WEBHOOK:
		payload, err = buildWebhookPayload(req)
	default:
		return &notifyv1.ChannelResult{Channel: ch, Success: false, Error: "unsupported channel"}
	}

	if err != nil {
		return &notifyv1.ChannelResult{Channel: ch, Success: false, Error: err.Error()}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.Webhook, bytes.NewReader(payload))
	if err != nil {
		return &notifyv1.ChannelResult{Channel: ch, Success: false, Error: err.Error()}
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return &notifyv1.ChannelResult{Channel: ch, Success: false, Error: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return &notifyv1.ChannelResult{Channel: ch, Success: false, Error: fmt.Sprintf("webhook returned %s", resp.Status)}
	}

	return &notifyv1.ChannelResult{Channel: ch, Success: true}
}

func (m *Module) buildApprisePayload(req *notifyv1.NotifyRequest, urls string) ([]byte, error) {
	body := map[string]any{
		"urls":   urls,
		"title":  req.GetTitle(),
		"body":   req.GetMessage(),
		"format": "text",
	}
	if p := m.severityToPriority(req.GetSeverity()); p != "" {
		body["priority"] = p
	}
	return json.Marshal(body)
}

func (m *Module) severityToPriority(severity string) string {
	switch severity {
	case "info":
		return "low"
	case "success":
		return "normal"
	case "warning":
		return "normal"
	case "error":
		return "high"
	default:
		return ""
	}
}

func buildDiscordPayload(req *notifyv1.NotifyRequest) ([]byte, error) {
	color := discordColor(req.GetSeverity())
	desc := req.GetMessage()
	if req.GetSourceModule() != "" {
		desc = fmt.Sprintf("**Source:** %s\n\n%s", req.GetSourceModule(), desc)
	}

	embed := map[string]any{
		"title":       req.GetTitle(),
		"description": desc,
		"color":       color,
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
	}

	var fields []map[string]any
	for k, v := range req.GetFields() {
		fields = append(fields, map[string]any{"name": k, "value": v, "inline": true})
	}
	if len(fields) > 0 {
		embed["fields"] = fields
	}

	return json.Marshal(map[string]any{"embeds": []any{embed}})
}

func buildSlackPayload(req *notifyv1.NotifyRequest) ([]byte, error) {
	color := slackColor(req.GetSeverity())
	text := fmt.Sprintf("*%s*\n%s", req.GetTitle(), req.GetMessage())

	var fields []map[string]any
	for k, v := range req.GetFields() {
		fields = append(fields, map[string]any{"title": k, "value": v, "short": true})
	}

	attachment := map[string]any{
		"color":    color,
		"text":     text,
		"fallback": req.GetTitle(),
		"ts":       time.Now().Unix(),
	}
	if len(fields) > 0 {
		attachment["fields"] = fields
	}

	return json.Marshal(map[string]any{
		"attachments": []any{attachment},
	})
}

func buildWebhookPayload(req *notifyv1.NotifyRequest) ([]byte, error) {
	return json.Marshal(map[string]any{
		"title":     req.GetTitle(),
		"message":   req.GetMessage(),
		"severity":  req.GetSeverity(),
		"source":    req.GetSourceModule(),
		"fields":    req.GetFields(),
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func discordColor(severity string) int {
	switch strings.ToLower(severity) {
	case "error":
		return 0xE74C3C
	case "warning":
		return 0xF39C12
	case "success":
		return 0x2ECC71
	default:
		return 0x3498DB
	}
}

func slackColor(severity string) string {
	switch strings.ToLower(severity) {
	case "error":
		return "danger"
	case "warning":
		return "warning"
	default:
		return "good"
	}
}

func (m *Module) Configure(ctx context.Context, req *notifyv1.ConfigureRequest) (*notifyv1.ConfigureResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := &channelConfig{
		Enabled:  true,
		Settings: req.GetSettings(),
	}
	if urls, ok := req.GetSettings()["urls"]; ok {
		cfg.URLs = urls
	}
	if url, ok := req.GetSettings()["webhook_url"]; ok {
		cfg.Webhook = url
	}
	m.channels[req.GetChannel()] = cfg
	return &notifyv1.ConfigureResponse{Configured: true}, nil
}

func (m *Module) Status(ctx context.Context, req *notifyv1.StatusRequest) (*notifyv1.StatusResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var statuses []*notifyv1.ChannelStatus
	for ch, cfg := range m.channels {
		statuses = append(statuses, &notifyv1.ChannelStatus{
			Channel:     ch,
			Enabled:     cfg.Enabled,
			Description: channelDescription(ch, cfg),
		})
	}
	if statuses == nil {
		statuses = []*notifyv1.ChannelStatus{}
	}
	return &notifyv1.StatusResponse{Channels: statuses}, nil
}

func channelDescription(ch notifyv1.Channel, cfg *channelConfig) string {
	desc := "disabled"
	if cfg.Enabled {
		desc = "enabled"
	}
	url := cfg.URLs
	if url == "" {
		url = cfg.Webhook
	}
	if url != "" {
		if len(url) > 60 {
			url = url[:60] + "..."
		}
		desc += " — " + url
	}
	return desc
}

var _ contracts.Module = (*Module)(nil)
