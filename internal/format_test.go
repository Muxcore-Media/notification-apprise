package internal

import (
	"encoding/json"
	"testing"

	notifyv1 "github.com/Muxcore-Media/contracts-notification/muxcore/notification/v1"
	"github.com/Muxcore-Media/core/pkg/contracts"
)

func TestFormatDownloadLifecycle(t *testing.T) {
	m := &Module{}

	disp, _ := json.Marshal(contracts.DownloadDispatchedPayload{Title: "Dune", DownloadProtocol: "torrent", Score: 10})
	title, msg, sev, _ := m.formatNotification(contracts.EventDownloadDispatched, disp)
	if title != "Download Dispatched" || msg != "Dune" || sev != notifyv1.Severity_SEVERITY_INFO {
		t.Fatalf("dispatched: %q %q %v", title, msg, sev)
	}

	done, _ := json.Marshal(contracts.DownloadEventPayload{ID: "d1", Name: "Dune.mkv", SavePath: "/dl/Dune"})
	title, msg, sev, fields := m.formatNotification(contracts.EventDownloadCompleted, done)
	if title != "Download Completed" || msg != "Dune.mkv" || sev != notifyv1.Severity_SEVERITY_SUCCESS {
		t.Fatalf("completed: %q %q %v", title, msg, sev)
	}
	if fields["Path"] != "/dl/Dune" {
		t.Fatalf("path field: %v", fields)
	}

	fail, _ := json.Marshal(contracts.DownloadEventPayload{ID: "d2", Name: "X", Error: "timeout"})
	title, msg, sev, _ = m.formatNotification(contracts.EventDownloadFailed, fail)
	if title != "Download Failed" || sev != notifyv1.Severity_SEVERITY_ERROR || msg != "X — timeout" {
		t.Fatalf("failed: %q %q %v", title, msg, sev)
	}
}

func TestFormatImportFailed(t *testing.T) {
	m := &Module{}
	imp, _ := json.Marshal(contracts.ImportFailedPayload{DownloadID: "d3", Path: "/dl/x", Error: "no video"})
	title, msg, sev, _ := m.formatNotification(contracts.EventImportFailed, imp)
	if title != "Import Failed" || sev != notifyv1.Severity_SEVERITY_ERROR || msg != "/dl/x — no video" {
		t.Fatalf("import: %q %q %v", title, msg, sev)
	}
}

func TestFormatMediaRequested(t *testing.T) {
	m := &Module{}
	movie, _ := json.Marshal(map[string]any{
		"title": "Dune", "year": 2021, "tmdb_id": 438631,
		"request_id": "req-1", "requested_by": "ender",
	})
	title, msg, sev, fields := m.formatNotification(contracts.EventMovieRequested, movie)
	if title != "Movie Requested" || msg != "Dune (2021)" || sev != notifyv1.Severity_SEVERITY_INFO {
		t.Fatalf("movie requested: %q %q %v", title, msg, sev)
	}
	if fields["Requester"] != "ender" || fields["TMDB ID"] != "438631" {
		t.Fatalf("fields: %v", fields)
	}
}

func TestFormatDownloadStarted(t *testing.T) {
	m := &Module{}
	started, _ := json.Marshal(contracts.DownloadEventPayload{ID: "d9", Name: "File.mkv", SavePath: "/dl"})
	title, msg, sev, fields := m.formatNotification(contracts.EventDownloadStarted, started)
	if title != "Download Started" || msg != "File.mkv" || sev != notifyv1.Severity_SEVERITY_INFO {
		t.Fatalf("started: %q %q %v", title, msg, sev)
	}
	if fields["Path"] != "/dl" {
		t.Fatalf("fields: %v", fields)
	}
}
