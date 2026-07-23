# notification-apprise

A MuxCore notification sidecar that delivers alerts through [Apprise](https://github.com/caronc/apprise) — a universal notification gateway supporting 100+ services (Slack, Telegram, Pushover, email, SMS, and many more) — plus optional direct Discord, Slack, and generic webhook channels.

## How it works

This module is a gRPC sidecar implementing the `NotificationProvider` contract (`NotificationService`: Notify, Configure, Status). Apprise destinations are posted to an Apprise API server; Discord, Slack, and generic webhook channels POST directly to their configured URLs. The module also subscribes to media events from core and forwards them as notifications.

## Configuration

| Env variable       | Default                 | Description                                         |
|--------------------|-------------------------|-----------------------------------------------------|
| `APPRISE_URL`      | `http://localhost:8000` | Base URL of the Apprise API server                  |
| `APPRISE_URLS`     | —                       | Semicolon-separated Apprise destination URLs        |
| `APPRISE_TOKEN`    | —                       | Bearer token for Apprise authentication             |
| `DISCORD_WEBHOOK`  | —                       | Discord webhook URL (direct channel)                |
| `SLACK_WEBHOOK`    | —                       | Slack incoming webhook URL (direct channel)         |
| `WEBHOOK_URL`      | —                       | Generic JSON webhook URL (direct channel)           |
| `NOTIFY_GRPC_ADDR` | `:9445`                 | gRPC listen address                                 |
| `MUXCORE_GRPC_ADDR`| `localhost:9090`        | Core mesh gRPC address                              |
| `MUXCORE_INSECURE_DISABLE_TLS` | —                  | Set `true` to disable TLS when dialing core (dev)   |
| `MUXCORE_MODULE_ID`| `notification-apprise`  | Module identity override (via module SDK)           |

## Key features

- **Multi-protocol gateway** — Apprise reaches any supported service; Discord/Slack/webhook channels send directly.
- **Severity mapping** — Apprise: `info` → low, `success`/`warning` → normal, `error` → high priority.
- **Dynamic configuration** — URLs and channels can be added at runtime via gRPC `Configure` calls.
- **Media event forwarding** — subscribes to movie/TV/download events from core and notifies configured channels.
