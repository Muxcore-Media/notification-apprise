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

	mediaevents "github.com/Muxcore-Media/contracts-media/events"
	notifyv1 "github.com/Muxcore-Media/contracts-notification/muxcore/notification/v1"
	"github.com/Muxcore-Media/core/pkg/contracts"
	eventsv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/events/v1"
	"github.com/Muxcore-Media/core/sdk/go/client"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
	"github.com/Muxcore-Media/core/sdk/go/module/netguard"
	manifest "github.com/Muxcore-Media/notification-apprise"
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
	client   *http.Client // webhooks: netguard UserURL profile
	// appriseClient talks to the admin-configured Apprise server (Integration profile).
	appriseClient *http.Client
	mc            *client.Client

	id           string
	dataDir      string
	moduleToken  string
	grpcAddr     string
	appriseURL   string
	appriseURLs  string
	appriseToken string
	grpcSrv      *grpc.Server
	lis          net.Listener
	prefs        eventPrefs
	eventMu      sync.Mutex
	eventCancels []context.CancelFunc
	stopCh       chan struct{}
}

type Config struct {
	ID             string
	DataDir        string
	GRPCAddr       string
	ModuleToken    string
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
		cfg.GRPCAddr = "127.0.0.1:9445"
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
		id:            cfg.ID,
		dataDir:       cfg.DataDir,
		moduleToken:   cfg.ModuleToken,
		grpcAddr:      cfg.GRPCAddr,
		appriseURL:    cfg.AppriseURL,
		appriseURLs:   cfg.AppriseURLs,
		appriseToken:  cfg.AppriseToken,
		client:        netguard.NewClient(netguard.UserURL, webhookOpts),
		appriseClient: netguard.NewClient(netguard.Integration, appriseOpts),
		channels:      map[notifyv1.Channel]*channelConfig{},
		prefs:         defaultEventPrefs(),
		stopCh:        make(chan struct{}),
	}
	if m.moduleToken == "" {
		m.moduleToken = moduleTokenFromEnv()
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
		Version:      modulesdk.ManifestVersion(manifest.ManifestJSON),
		Roles:        []string{"notification"},
		Description:  "Apprise multi-platform notifications with Discord, Slack, and webhook channel support",
		Author:       "MuxCore",
		Capabilities: []string{"notification", "notification.apprise", "settings"},
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
	if err := os.MkdirAll(m.dataDirPath(), 0700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	if err := m.loadPersisted(); err != nil {
		return err
	}
	lis, err := net.Listen("tcp", m.grpcAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", m.grpcAddr, err)
	}
	m.lis = lis
	slog.Info("notification-apprise initialized", "addr", m.grpcAddr, "data_dir", m.dataDirPath())
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	m.grpcSrv = grpc.NewServer(grpc.UnaryInterceptor(authUnaryInterceptor(m.moduleToken)))
	notifyv1.RegisterNotificationServiceServer(m.grpcSrv, m)
	modulesdk.RegisterSettings(m.grpcSrv, m.id, m)
	go func() {
		slog.Info("notification-apprise gRPC service started", "addr", m.grpcAddr)
		if err := m.grpcSrv.Serve(m.lis); err != nil {
			slog.Error("notification-apprise gRPC serve error", "error", err)
		}
	}()
	go m.connectCoreAndSubscribe(ctx)
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	select {
	case <-m.stopCh:
	default:
		close(m.stopCh)
	}
	m.cancelEventStreams()
	m.mu.Lock()
	if m.mc != nil {
		_ = m.mc.Close()
		m.mc = nil
	}
	m.mu.Unlock()
	if m.grpcSrv != nil {
		m.grpcSrv.GracefulStop()
	}
	m.client.CloseIdleConnections()
	slog.Info("notification-apprise stopped")
	return nil
}

func (m *Module) cancelEventStreams() {
	m.eventMu.Lock()
	defer m.eventMu.Unlock()
	for _, cancel := range m.eventCancels {
		cancel()
	}
	m.eventCancels = nil
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

func (m *Module) connectCoreAndSubscribe(ctx context.Context) {
	meshAddr := os.Getenv("MUXCORE_GRPC_ADDR")
	if meshAddr == "" {
		meshAddr = "localhost:9090"
	}
	insecureMode := os.Getenv("MUXCORE_INSECURE_DISABLE_TLS") == "true" || os.Getenv("MUXCORE_GRPC_INSECURE") == "true"
	var opts []client.Option
	if insecureMode {
		opts = append(opts, client.WithInsecure())
	}

	backoff := time.Second
	for {
		select {
		case <-m.stopCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		c, err := client.Dial(meshAddr, opts...)
		if err != nil {
			slog.Warn("notification-apprise: dial core failed, retrying", "error", err, "backoff", backoff)
			select {
			case <-m.stopCh:
				return
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}

		m.mu.Lock()
		if m.mc != nil {
			_ = m.mc.Close()
		}
		m.mc = c
		m.mu.Unlock()
		slog.Info("notification-apprise: connected to core mesh", "addr", meshAddr)
		m.subscribeToMediaEvents(ctx)

		m.mu.Lock()
		if m.mc != nil {
			_ = m.mc.Close()
			m.mc = nil
		}
		m.mu.Unlock()
		m.cancelEventStreams()
		backoff = time.Second
	}
}

func (m *Module) subscribeToMediaEvents(ctx context.Context) {
	m.mu.RLock()
	mc := m.mc
	m.mu.RUnlock()
	if mc == nil {
		return
	}

	eventTypes := []string{
		contracts.EventMovieAdded,
		contracts.EventMovieRemoved,
		mediaevents.EventMovieRequested,
		contracts.EventMovieFileAdded,
		contracts.EventTVAdded,
		contracts.EventTVRemoved,
		mediaevents.EventTVRequested,
		contracts.EventTVEpisodeFileAdded,
		contracts.EventFileImported,
		contracts.EventImportFailed,
		contracts.EventDownloadDispatched,
		contracts.EventDownloadStarted,
		contracts.EventDownloadCompleted,
		contracts.EventDownloadFailed,
	}

	for _, et := range eventTypes {
		select {
		case <-m.stopCh:
			return
		case <-ctx.Done():
			return
		default:
		}
		subCtx, cancel := context.WithCancel(ctx)
		ch, subCancel, err := mc.Events.Subscribe(subCtx, et)
		if err != nil {
			cancel()
			slog.Warn("subscribe to event", "type", et, "error", err)
			continue
		}
		streamCancel := func() {
			subCancel()
			cancel()
		}
		m.eventMu.Lock()
		m.eventCancels = append(m.eventCancels, streamCancel)
		m.eventMu.Unlock()
		go m.handleEventStream(et, ch, streamCancel)
		slog.Info("subscribed to events", "type", et)
	}

	<-m.stopCh
}

func (m *Module) handleEventStream(eventType string, ch <-chan *eventsv1.Event, cancel context.CancelFunc) {
	defer cancel()
	for evt := range ch {
		m.mu.RLock()
		allowed := m.prefs.allows(eventType)
		m.mu.RUnlock()
		if !allowed {
			continue
		}
		title, message, severity, fields := m.formatNotification(eventType, evt.Payload)
		if title == "" {
			continue
		}
		if _, err := m.Notify(context.Background(), &notifyv1.NotifyRequest{
			Title:        title,
			Message:      message,
			Severity:     severity,
			SourceModule: evt.Source,
			Fields:       fields,
		}); err != nil {
			slog.Warn("notification-apprise: notify failed", "event", eventType, "error", err)
		}
	}
}

func (m *Module) formatNotification(eventType string, payload []byte) (title, message string, severity notifyv1.Severity, fields map[string]string) {
	fields = map[string]string{}

	switch eventType {
	case contracts.EventMovieAdded:
		var p contracts.MovieAddedPayload
		if json.Unmarshal(payload, &p) != nil {
			return
		}
		return "Movie Added", p.Title, notifyv1.Severity_SEVERITY_SUCCESS, map[string]string{
			"TMDB ID": strconv.Itoa(int(p.TMDBID)), "Movie ID": p.MovieID,
		}

	case contracts.EventMovieRemoved:
		var p contracts.MovieRemovedPayload
		if json.Unmarshal(payload, &p) != nil {
			return
		}
		return "Movie Removed", p.Title, notifyv1.Severity_SEVERITY_WARNING, map[string]string{"TMDB ID": strconv.Itoa(int(p.TMDBID))}

	case contracts.EventMovieFileAdded:
		var p contracts.MovieFileAddedPayload
		if json.Unmarshal(payload, &p) != nil {
			return
		}
		return "Movie File Added", p.FilePath, notifyv1.Severity_SEVERITY_INFO, map[string]string{
			"Quality": p.Quality, "Movie ID": p.MovieID,
		}

	case contracts.EventTVAdded:
		var p contracts.TVAddedPayload
		if json.Unmarshal(payload, &p) != nil {
			return
		}
		return "TV Show Added", p.Name, notifyv1.Severity_SEVERITY_SUCCESS, map[string]string{
			"TMDB ID": strconv.Itoa(int(p.TMDBID)), "Series ID": p.SeriesID,
		}

	case contracts.EventTVRemoved:
		var p contracts.TVRemovedPayload
		if json.Unmarshal(payload, &p) != nil {
			return
		}
		return "TV Show Removed", p.SeriesID, notifyv1.Severity_SEVERITY_WARNING, nil

	case contracts.EventTVEpisodeFileAdded:
		var p contracts.TVEpisodeFileAddedPayload
		if json.Unmarshal(payload, &p) != nil {
			return
		}
		return "Episode File Added", p.FilePath, notifyv1.Severity_SEVERITY_INFO, map[string]string{
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
		return label, fmt.Sprintf("%s (%d)", p.Title, p.Year), notifyv1.Severity_SEVERITY_SUCCESS, map[string]string{
			"Type": p.MediaType, "Quality": p.Quality, "Path": p.DestinationPath,
		}

	case contracts.EventDownloadDispatched:
		var p contracts.DownloadDispatchedPayload
		if json.Unmarshal(payload, &p) != nil {
			return
		}
		return "Download Dispatched", p.Title, notifyv1.Severity_SEVERITY_INFO, map[string]string{
			"Protocol": p.DownloadProtocol, "Score": strconv.Itoa(int(p.Score)),
		}

	case contracts.EventDownloadStarted:
		var p contracts.DownloadEventPayload
		if json.Unmarshal(payload, &p) != nil {
			return
		}
		name := p.Name
		if name == "" {
			name = p.ID
		}
		return "Download Started", name, notifyv1.Severity_SEVERITY_INFO, map[string]string{
			"Download ID": p.ID, "Path": p.SavePath,
		}

	case mediaevents.EventMovieRequested:
		return formatMediaRequested("Movie Requested", payload)

	case mediaevents.EventTVRequested:
		return formatMediaRequested("TV Show Requested", payload)

	case contracts.EventDownloadCompleted:
		var p contracts.DownloadEventPayload
		if json.Unmarshal(payload, &p) != nil {
			return
		}
		name := p.Name
		if name == "" {
			name = p.ID
		}
		return "Download Completed", name, notifyv1.Severity_SEVERITY_SUCCESS, map[string]string{
			"Download ID": p.ID, "Path": p.SavePath,
		}

	case contracts.EventDownloadFailed:
		var p contracts.DownloadEventPayload
		if json.Unmarshal(payload, &p) != nil {
			return
		}
		name := p.Name
		if name == "" {
			name = p.ID
		}
		msg := name
		if p.Error != "" {
			msg = fmt.Sprintf("%s — %s", name, p.Error)
		}
		return "Download Failed", msg, notifyv1.Severity_SEVERITY_ERROR, map[string]string{
			"Download ID": p.ID, "Error": p.Error,
		}

	case contracts.EventImportFailed:
		var p contracts.ImportFailedPayload
		if json.Unmarshal(payload, &p) != nil {
			return
		}
		msg := p.Path
		if msg == "" {
			msg = p.DownloadID
		}
		if p.Error != "" {
			msg = fmt.Sprintf("%s — %s", msg, p.Error)
		}
		return "Import Failed", msg, notifyv1.Severity_SEVERITY_ERROR, map[string]string{
			"Download ID": p.DownloadID, "Path": p.Path, "Error": p.Error,
		}
	}

	return
}

func formatMediaRequested(label string, payload []byte) (title, message string, severity notifyv1.Severity, fields map[string]string) {
	var p map[string]any
	if json.Unmarshal(payload, &p) != nil {
		return
	}
	titleStr, _ := p["title"].(string)
	if titleStr == "" {
		return
	}
	message = titleStr
	if yr, ok := p["year"].(float64); ok && yr > 0 {
		message = fmt.Sprintf("%s (%d)", titleStr, int(yr))
	}
	fields = map[string]string{}
	if requester, _ := p["requested_by"].(string); requester != "" {
		fields["Requester"] = requester
	} else if approved, _ := p["approved_by"].(string); approved != "" {
		fields["Requester"] = approved
	}
	if id, _ := p["request_id"].(string); id != "" {
		fields["Request ID"] = id
	}
	if tmdb, ok := p["tmdb_id"].(float64); ok && tmdb > 0 {
		fields["TMDB ID"] = strconv.Itoa(int(tmdb))
	}
	return label, message, notifyv1.Severity_SEVERITY_INFO, fields
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

	urlBase, token := m.getAppriseEndpoint()
	url := urlBase + "/notify"
	if err := validateAppriseURL(url); err != nil {
		return &notifyv1.ChannelResult{Channel: notifyv1.Channel_CHANNEL_APPRISE, Success: false, Error: err.Error()}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return &notifyv1.ChannelResult{
			Channel: notifyv1.Channel_CHANNEL_APPRISE,
			Success: false,
			Error:   err.Error(),
		}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := m.appriseClient.Do(httpReq)
	if err != nil {
		return &notifyv1.ChannelResult{
			Channel: notifyv1.Channel_CHANNEL_APPRISE,
			Success: false,
			Error:   err.Error(),
		}
	}
	defer func() { _ = resp.Body.Close() }()

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
	defer func() { _ = resp.Body.Close() }()

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

func (m *Module) severityToPriority(severity notifyv1.Severity) string {
	switch severity {
	case notifyv1.Severity_SEVERITY_INFO:
		return "low"
	case notifyv1.Severity_SEVERITY_SUCCESS, notifyv1.Severity_SEVERITY_WARNING:
		return "normal"
	case notifyv1.Severity_SEVERITY_ERROR:
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
		"severity":  severityLabel(req.GetSeverity()),
		"source":    req.GetSourceModule(),
		"fields":    req.GetFields(),
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func discordColor(severity notifyv1.Severity) int {
	switch severity {
	case notifyv1.Severity_SEVERITY_ERROR:
		return 0xE74C3C
	case notifyv1.Severity_SEVERITY_WARNING:
		return 0xF39C12
	case notifyv1.Severity_SEVERITY_SUCCESS:
		return 0x2ECC71
	default:
		return 0x3498DB
	}
}

func slackColor(severity notifyv1.Severity) string {
	switch severity {
	case notifyv1.Severity_SEVERITY_ERROR:
		return "danger"
	case notifyv1.Severity_SEVERITY_WARNING:
		return "warning"
	default:
		return "good"
	}
}

func severityLabel(severity notifyv1.Severity) string {
	switch severity {
	case notifyv1.Severity_SEVERITY_ERROR:
		return "ERROR"
	case notifyv1.Severity_SEVERITY_WARNING:
		return "WARNING"
	case notifyv1.Severity_SEVERITY_SUCCESS:
		return "SUCCESS"
	case notifyv1.Severity_SEVERITY_INFO:
		return "INFO"
	default:
		return ""
	}
}

func (m *Module) Configure(ctx context.Context, req *notifyv1.ConfigureRequest) (*notifyv1.ConfigureResponse, error) {
	ch := req.GetChannel()
	if ch == notifyv1.Channel_CHANNEL_UNSPECIFIED {
		return nil, fmt.Errorf("channel must be specified")
	}
	if ch == notifyv1.Channel_CHANNEL_EMAIL {
		return nil, fmt.Errorf("email channel is not supported by notification-apprise")
	}

	settings := req.GetSettings()
	if settings == nil {
		settings = map[string]string{}
	}

	switch ch {
	case notifyv1.Channel_CHANNEL_APPRISE:
		urls := strings.TrimSpace(settings["urls"])
		if urls == "" {
			m.mu.Lock()
			delete(m.channels, ch)
			m.mu.Unlock()
			if err := m.persistSettings(); err != nil {
				return &notifyv1.ConfigureResponse{Configured: false}, err
			}
			return &notifyv1.ConfigureResponse{Configured: true}, nil
		}
		m.mu.Lock()
		m.channels[ch] = &channelConfig{
			Enabled: true,
			URLs:    urls,
			Settings: map[string]string{
				"type": "apprise",
				"urls": urls,
			},
		}
		m.mu.Unlock()
	case notifyv1.Channel_CHANNEL_DISCORD, notifyv1.Channel_CHANNEL_SLACK, notifyv1.Channel_CHANNEL_WEBHOOK:
		url := strings.TrimSpace(settings["webhook_url"])
		if url == "" {
			m.mu.Lock()
			delete(m.channels, ch)
			m.mu.Unlock()
			if err := m.persistSettings(); err != nil {
				return &notifyv1.ConfigureResponse{Configured: false}, err
			}
			return &notifyv1.ConfigureResponse{Configured: true}, nil
		}
		if err := validateWebhookURL(ctx, url); err != nil {
			return &notifyv1.ConfigureResponse{Configured: false}, err
		}
		typ := "generic"
		switch ch {
		case notifyv1.Channel_CHANNEL_DISCORD:
			typ = "discord"
		case notifyv1.Channel_CHANNEL_SLACK:
			typ = "slack"
		}
		m.mu.Lock()
		m.channels[ch] = &channelConfig{
			Enabled:  true,
			Webhook:  url,
			Settings: map[string]string{"type": typ, "webhook_url": url},
		}
		m.mu.Unlock()
	default:
		return nil, fmt.Errorf("unsupported channel %s", ch)
	}

	if err := m.persistSettings(); err != nil {
		return &notifyv1.ConfigureResponse{Configured: false}, err
	}
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
	if cfg.URLs != "" {
		desc += " — " + modulesdk.MaskSecret(cfg.URLs)
	} else if cfg.Webhook != "" {
		desc += " — " + modulesdk.MaskSecret(cfg.Webhook)
	}
	_ = ch
	return desc
}

var _ contracts.Module = (*Module)(nil)
