package internal

import (
	"context"
	"fmt"
	"strings"

	notifyv1 "github.com/Muxcore-Media/contracts-notification/muxcore/notification/v1"
	"github.com/Muxcore-Media/core/pkg/contracts"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
)

func (m *Module) Settings() []contracts.SettingDef {
	return m.settingsDefs()
}

func (m *Module) UpdateSetting(key, value string) error {
	err := m.updateSetting(key, value)
	if err != nil {
		return err
	}
	return m.persistSettings()
}

func (m *Module) settingsDefs() []contracts.SettingDef {
	m.mu.RLock()
	defer m.mu.RUnlock()

	discord, slack, webhook, appriseURLs := "", "", "", ""
	if c := m.channels[notifyv1.Channel_CHANNEL_DISCORD]; c != nil {
		discord = c.Webhook
	}
	if c := m.channels[notifyv1.Channel_CHANNEL_SLACK]; c != nil {
		slack = c.Webhook
	}
	if c := m.channels[notifyv1.Channel_CHANNEL_WEBHOOK]; c != nil {
		webhook = c.Webhook
	}
	if c := m.channels[notifyv1.Channel_CHANNEL_APPRISE]; c != nil {
		appriseURLs = c.URLs
	}
	if appriseURLs == "" {
		appriseURLs = m.appriseURLs
	}

	defs := []contracts.SettingDef{
		{Key: "apprise_url", Label: "Apprise Base URL", Type: contracts.SettingTypeString,
			Value: m.appriseURL, Group: "Apprise", Description: "APPRISE_URL"},
		{Key: "apprise_urls", Label: "Apprise Notification URLs", Type: contracts.SettingTypeSecret,
			Value: modulesdk.MaskSecret(appriseURLs), Group: "Apprise", Description: "APPRISE_URLS"},
		{Key: "apprise_token", Label: "Apprise Bearer Token", Type: contracts.SettingTypeSecret,
			Value: modulesdk.MaskSecret(m.appriseToken), Group: "Apprise", Description: "APPRISE_TOKEN"},
		{Key: "discord_webhook", Label: "Discord Webhook URL", Type: contracts.SettingTypeSecret,
			Value: modulesdk.MaskSecret(discord), Group: "Channels", Description: "DISCORD_WEBHOOK"},
		{Key: "slack_webhook", Label: "Slack Webhook URL", Type: contracts.SettingTypeSecret,
			Value: modulesdk.MaskSecret(slack), Group: "Channels", Description: "SLACK_WEBHOOK"},
		{Key: "webhook_url", Label: "Generic Webhook URL", Type: contracts.SettingTypeSecret,
			Value: modulesdk.MaskSecret(webhook), Group: "Channels", Description: "WEBHOOK_URL"},
	}

	eventSettings := []struct {
		key  string
		val  bool
		desc string
	}{
		{"notify_requested", m.prefs.NotifyRequested, "media.movie.requested / media.tv.requested"},
		{"notify_file_added", m.prefs.NotifyFileAdded, "movie/episode file added and file imported"},
		{"notify_import_failed", m.prefs.NotifyImportFailed, "import.failed"},
		{"notify_download_failed", m.prefs.NotifyDownloadFailed, "download.failed"},
		{"notify_download_started", m.prefs.NotifyDownloadStarted, "download.started"},
		{"notify_download_completed", m.prefs.NotifyDownloadCompleted, "download.completed"},
		{"notify_download_dispatched", m.prefs.NotifyDownloadDispatched, "download.dispatched"},
		{"notify_media_added", m.prefs.NotifyMediaAdded, "movie/tv added"},
		{"notify_media_removed", m.prefs.NotifyMediaRemoved, "movie/tv removed"},
	}
	for _, es := range eventSettings {
		defs = append(defs, contracts.SettingDef{
			Key: es.key, Label: es.key, Type: contracts.SettingTypeBool,
			Value: boolString(es.val), Group: "Events", Description: es.desc,
		})
	}
	return defs
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func (m *Module) updateSetting(key, value string) error {
	value = strings.TrimSpace(value)
	switch key {
	case "apprise_url", "APPRISE_URL":
		if value == "" {
			return fmt.Errorf("apprise_url must not be empty")
		}
		if err := validateAppriseURL(value); err != nil {
			return err
		}
		m.mu.Lock()
		m.appriseURL = strings.TrimRight(value, "/")
		m.mu.Unlock()
		return nil
	case "apprise_urls", "APPRISE_URLS":
		if value == "********" {
			return nil
		}
		m.mu.Lock()
		m.appriseURLs = value
		if value == "" {
			delete(m.channels, notifyv1.Channel_CHANNEL_APPRISE)
		} else {
			m.channels[notifyv1.Channel_CHANNEL_APPRISE] = &channelConfig{
				Enabled: true,
				URLs:    value,
				Settings: map[string]string{
					"type": "apprise",
					"urls": value,
				},
			}
		}
		m.mu.Unlock()
		return nil
	case "apprise_token", "APPRISE_TOKEN":
		if value == "********" {
			return nil
		}
		m.mu.Lock()
		m.appriseToken = value
		m.mu.Unlock()
		return nil
	case "discord_webhook", "DISCORD_WEBHOOK":
		if value == "********" {
			return nil
		}
		return m.setWebhookChannel(notifyv1.Channel_CHANNEL_DISCORD, value, "discord")
	case "slack_webhook", "SLACK_WEBHOOK":
		if value == "********" {
			return nil
		}
		return m.setWebhookChannel(notifyv1.Channel_CHANNEL_SLACK, value, "slack")
	case "webhook_url", "WEBHOOK_URL":
		if value == "********" {
			return nil
		}
		return m.setWebhookChannel(notifyv1.Channel_CHANNEL_WEBHOOK, value, "generic")
	default:
		if set, ok := eventPrefKey(key); ok {
			enabled, err := parseBoolSetting(value)
			if err != nil {
				return err
			}
			m.mu.Lock()
			set(&m.prefs, enabled)
			m.mu.Unlock()
			return nil
		}
		return fmt.Errorf("unknown setting %q", key)
	}
}

func (m *Module) setWebhookChannel(ch notifyv1.Channel, url, typ string) error {
	if url != "" {
		if err := validateWebhookURL(context.Background(), url); err != nil {
			return err
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if url == "" {
		delete(m.channels, ch)
		return nil
	}
	m.channels[ch] = &channelConfig{
		Enabled:  true,
		Webhook:  url,
		Settings: map[string]string{"type": typ},
	}
	return nil
}

func (m *Module) getAppriseEndpoint() (baseURL, token string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.appriseURL, m.appriseToken
}
