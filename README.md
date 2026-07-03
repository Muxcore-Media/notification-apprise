# notification-apprise

A MuxCore notification sidecar that delivers alerts through [Apprise](https://github.com/caronc/apprise) — a universal notification gateway supporting 100+ services (Slack, Telegram, Pushover, email, SMS, and many more).

## How it works

This module is a gRPC sidecar implementing the `NotificationProvider` contract. Instead of talking to a single provider, it delegates all delivery to an Apprise API server. Apprise handles routing to the configured destination URLs.

## Configuration

| Env variable     | Default                  | Description                              |
|------------------|--------------------------|------------------------------------------|
| `APPRISE_URL`    | `http://localhost:8000`  | Base URL of the Apprise API server       |
| `APPRISE_URLS`   | —                        | Semicolon-separated Apprise destination URLs |
| `APPRISE_TOKEN`  | —                        | Bearer token for Apprise authentication  |
| `NOTIFY_GRPC_ADDR` | `:9445`               | gRPC listen address                      |

## Key features

- **Multi-protocol gateway** — a single module can reach any Apprise-supported service.
- **Severity mapping** — `info` → low, `success`/`warning` → normal, `error` → high priority.
- **Dynamic configuration** — URLs and channels can be added at runtime via gRPC `Configure` calls.
- **Zero provider code** — all provider-specific logic lives in the Apprise server itself.
