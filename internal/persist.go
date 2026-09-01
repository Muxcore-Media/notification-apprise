package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	notifyv1 "github.com/Muxcore-Media/contracts-notification/muxcore/notification/v1"
)

type persistedChannel struct {
	Enabled  bool              `json:"enabled"`
	URLs     string            `json:"urls,omitempty"`
	Webhook  string            `json:"webhook,omitempty"`
	Settings map[string]string `json:"settings,omitempty"`
}

type persistedSettings struct {
	AppriseURL   string                      `json:"apprise_url,omitempty"`
	AppriseToken string                      `json:"apprise_token,omitempty"`
	Channels     map[string]persistedChannel `json:"channels"`
	EventPrefs   *eventPrefs                 `json:"event_prefs,omitempty"`
}

func (m *Module) dataDirPath() string {
	if m.dataDir != "" {
		return m.dataDir
	}
	root := os.Getenv("MUXCORE_DATA")
	if root == "" {
		root = "data"
	}
	return filepath.Join(root, m.id)
}

func (m *Module) settingsPath() string {
	return filepath.Join(m.dataDirPath(), "settings.json")
}

func (m *Module) loadPersisted() error {
	path := m.settingsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return m.persistSettings()
		}
		return err
	}
	var ps persistedSettings
	if err := json.Unmarshal(data, &ps); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if ps.AppriseURL != "" {
		m.appriseURL = ps.AppriseURL
	}
	if ps.AppriseToken != "" {
		m.appriseToken = ps.AppriseToken
	}
	if len(ps.Channels) > 0 {
		m.channels = map[notifyv1.Channel]*channelConfig{}
		for name, pc := range ps.Channels {
			ch, ok := channelFromPersistName(name)
			if !ok {
				continue
			}
			settings := pc.Settings
			if settings == nil {
				settings = map[string]string{}
			}
			m.channels[ch] = &channelConfig{
				Enabled:  pc.Enabled,
				URLs:     pc.URLs,
				Webhook:  pc.Webhook,
				Settings: settings,
			}
		}
	}
	if ps.EventPrefs != nil {
		m.prefs = *ps.EventPrefs
	}
	return nil
}

func (m *Module) persistSettings() error {
	m.mu.RLock()
	ps := persistedSettings{
		AppriseURL:   m.appriseURL,
		AppriseToken: m.appriseToken,
		Channels:     map[string]persistedChannel{},
	}
	prefs := m.prefs
	ps.EventPrefs = &prefs
	for ch, cfg := range m.channels {
		if cfg == nil {
			continue
		}
		settings := cfg.Settings
		if settings == nil {
			settings = map[string]string{}
		}
		ps.Channels[channelPersistName(ch)] = persistedChannel{
			Enabled:  cfg.Enabled,
			URLs:     cfg.URLs,
			Webhook:  cfg.Webhook,
			Settings: settings,
		}
	}
	m.mu.RUnlock()

	if err := os.MkdirAll(m.dataDirPath(), 0700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	data, err := json.MarshalIndent(ps, "", "  ")
	if err != nil {
		return err
	}
	path := m.settingsPath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func channelPersistName(ch notifyv1.Channel) string {
	return ch.String()
}

func channelFromPersistName(name string) (notifyv1.Channel, bool) {
	v, ok := notifyv1.Channel_value[name]
	if !ok {
		return notifyv1.Channel_CHANNEL_UNSPECIFIED, false
	}
	ch := notifyv1.Channel(v)
	if ch == notifyv1.Channel_CHANNEL_UNSPECIFIED {
		return ch, false
	}
	return ch, true
}
