# notification-apprise

A MuxCore notification sidecar that delivers alerts through [Apprise](https://github.com/caronc/apprise) — a universal notification gateway supporting 100+ services (Slack, Telegram, Pushover, email, SMS, and many more) — plus optional direct Discord, Slack, and generic webhook channels.

## How it works

gRPC sidecar implementing the `NotificationProvider` contract (`Notify`, `Configure`, `Status`). Apprise destinations are posted to an Apprise API server; Discord, Slack, and generic webhook channels POST directly to their configured URLs.

Also dials the core mesh (with backoff until connected) and subscribes to media/download events, formatting them into notifications. Per-event toggles can mute individual event types.

Channel and event preferences persist to `$MUXCORE_DATA/notification-apprise/settings.json` (mode `0600`).

## MVP host

Enable on the soak stack with `MVP_ENABLE_NOTIFICATION_APPRISE=1` in `_mvp/.env` or vault Nix config. Runs alongside `notification-default` as an optional extra sink.

## Configuration

| Env variable | Default | Description |
|--------------|---------|-------------|
| `NOTIFY_GRPC_ADDR` | `127.0.0.1:9445` | Notification gRPC listen address (localhost-only by default) |
| `NOTIFY_MODULE_TOKEN` | — | Bearer token for unauthenticated gRPC clients (also `MUXCORE_MODULE_TOKEN`) |
| `MUXCORE_DATA` | `data` | Data root; settings persist under `<data>/notification-apprise/` |
| `APPRISE_URL` | `http://localhost:8000` | Base URL of the Apprise API server |
| `APPRISE_URLS` | — | Semicolon-separated Apprise destination URLs |
| `APPRISE_TOKEN` | — | Bearer token for Apprise authentication |
| `DISCORD_WEBHOOK` | — | Discord webhook URL (direct channel) |
| `SLACK_WEBHOOK` | — | Slack incoming webhook URL (direct channel) |
| `WEBHOOK_URL` | — | Generic JSON webhook URL (direct channel) |
| `MUXCORE_MODULE_ID` | `notification-apprise` | Module ID (SDK) |
| `MUXCORE_GRPC_ADDR` | `localhost:9090` | Core mesh address |
| `MUXCORE_INSECURE_DISABLE_TLS` | unset | Set `true` to disable TLS for mesh/SDK |

`Health` passes when at least one notification channel is configured (`APPRISE_URLS`, `DISCORD_WEBHOOK`, `SLACK_WEBHOOK`, or `WEBHOOK_URL`).

### Settings (admin-ui / mesh)

**Apprise:** `apprise_url`, `apprise_urls`, `apprise_token`

**Channels:** `discord_webhook`, `slack_webhook`, `webhook_url`

**Event toggles:** `notify_requested`, `notify_file_added`, `notify_import_failed`, `notify_download_failed`, `notify_download_started`, `notify_download_completed`, `notify_download_dispatched`, `notify_media_added`, `notify_media_removed` (boolean; default on).

Settings persist across restarts. Runtime `Configure` via gRPC also persists channel changes.

Webhook settings and `Configure` reject non-https URLs and link-local/private targets.

### Runtime `Configure`

`Configure` validates channel input (rejects `CHANNEL_UNSPECIFIED` and `CHANNEL_EMAIL`; Apprise requires `urls`; Discord/Slack/webhook require `webhook_url`). Empty `urls` or `webhook_url` clears/disables the channel.

## Key features

- **Multi-protocol gateway** — Apprise reaches any supported service; Discord/Slack/webhook channels send directly.
- **Severity mapping** — Apprise: info → low, success/warning → normal, error → high priority.
- **Authenticated gRPC** — mesh identity or module token on `Notify`/`Configure`/`Status`.
- **Media event bridge** — movie/TV requests, downloads, imports, library changes.
- **Capabilities** — `notification`, `notification.apprise`, `settings` (`NotificationProvider` v0.1.0).

## Build

```bash
make build
make test
make lint
```

Docker image builds from module root (`docker build .`); no umbrella sibling COPY required.
