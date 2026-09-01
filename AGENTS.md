# AGENTS.md — notification-apprise

MuxCore sidecar module (`notification-apprise`). Workspace deploy and SSH: [`../AGENTS.md`](../AGENTS.md). Default ports: [`_mvp/PORTS.md`](../_mvp/PORTS.md).

## Module identity

| Field | Value |
|-------|-------|
| Directory | `notification-apprise` |
| Capabilities | `notification`, `notification.apprise`, `settings` |
| Contracts | `NotificationProvider` v0.1.0 (`contracts-notification`) |

## Agent rules

- Modules run as gRPC sidecars; capabilities are the security boundary.
- TLS required in production (`MUXCORE_INSECURE_DISABLE_TLS` is dev-only).
- Match existing Go patterns; run `gofmt` and package tests before finishing.
- Cross-module events: prefer `github.com/Muxcore-Media/contracts-media/events` over deprecated `core/pkg/contracts` aliases.
- Do not edit polluted workspace dumps (see `MASTER-ROADMAP.md` Appendix H).

## Build

```bash
cd notification-apprise
nix-shell -p go --run 'go test ./...'
```

Settings persist to `$MUXCORE_DATA/notification-apprise/settings.json`. Optional on vault: `MVP_ENABLE_NOTIFICATION_APPRISE=1`.
