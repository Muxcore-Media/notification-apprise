package internal

import (
	"fmt"
	"strings"

	"github.com/Muxcore-Media/core/pkg/contracts"
)

type eventPrefs struct {
	NotifyRequested          bool `json:"notify_requested"`
	NotifyFileAdded          bool `json:"notify_file_added"`
	NotifyImportFailed       bool `json:"notify_import_failed"`
	NotifyDownloadFailed     bool `json:"notify_download_failed"`
	NotifyDownloadStarted    bool `json:"notify_download_started"`
	NotifyDownloadCompleted  bool `json:"notify_download_completed"`
	NotifyDownloadDispatched bool `json:"notify_download_dispatched"`
	NotifyMediaAdded         bool `json:"notify_media_added"`
	NotifyMediaRemoved       bool `json:"notify_media_removed"`
}

func defaultEventPrefs() eventPrefs {
	return eventPrefs{
		NotifyRequested:          true,
		NotifyFileAdded:          true,
		NotifyImportFailed:       true,
		NotifyDownloadFailed:     true,
		NotifyDownloadStarted:    true,
		NotifyDownloadCompleted:  true,
		NotifyDownloadDispatched: true,
		NotifyMediaAdded:         true,
		NotifyMediaRemoved:       true,
	}
}

func (p eventPrefs) allows(eventType string) bool {
	switch eventType {
	case contracts.EventMovieRequested, contracts.EventTVRequested:
		return p.NotifyRequested
	case contracts.EventMovieFileAdded, contracts.EventTVEpisodeFileAdded, contracts.EventFileImported:
		return p.NotifyFileAdded
	case contracts.EventImportFailed:
		return p.NotifyImportFailed
	case contracts.EventDownloadFailed:
		return p.NotifyDownloadFailed
	case contracts.EventDownloadStarted:
		return p.NotifyDownloadStarted
	case contracts.EventDownloadCompleted:
		return p.NotifyDownloadCompleted
	case contracts.EventDownloadDispatched:
		return p.NotifyDownloadDispatched
	case contracts.EventMovieAdded, contracts.EventTVAdded:
		return p.NotifyMediaAdded
	case contracts.EventMovieRemoved, contracts.EventTVRemoved:
		return p.NotifyMediaRemoved
	default:
		return true
	}
}

func eventPrefKey(key string) (func(*eventPrefs, bool), bool) {
	switch key {
	case "notify_requested":
		return func(p *eventPrefs, v bool) { p.NotifyRequested = v }, true
	case "notify_file_added":
		return func(p *eventPrefs, v bool) { p.NotifyFileAdded = v }, true
	case "notify_import_failed":
		return func(p *eventPrefs, v bool) { p.NotifyImportFailed = v }, true
	case "notify_download_failed":
		return func(p *eventPrefs, v bool) { p.NotifyDownloadFailed = v }, true
	case "notify_download_started":
		return func(p *eventPrefs, v bool) { p.NotifyDownloadStarted = v }, true
	case "notify_download_completed":
		return func(p *eventPrefs, v bool) { p.NotifyDownloadCompleted = v }, true
	case "notify_download_dispatched":
		return func(p *eventPrefs, v bool) { p.NotifyDownloadDispatched = v }, true
	case "notify_media_added":
		return func(p *eventPrefs, v bool) { p.NotifyMediaAdded = v }, true
	case "notify_media_removed":
		return func(p *eventPrefs, v bool) { p.NotifyMediaRemoved = v }, true
	default:
		return nil, false
	}
}

func parseBoolSetting(v string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean %q", v)
	}
}
