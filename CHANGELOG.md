# Changelog

## [0.1.9] - 2026-10-05


### Security
- NFR-SEC-009 / RULE-VAL-2: webhook (Discord/Slack/generic) URLs are checked with netguard `UserURL` (https only; private, loopback, link-local and metadata targets blocked) at configure time, on load of persisted settings (unsafe persisted webhooks/apprise_url are ignored), and again at send time via a dial-time/redirect-guarded client (DNS rebinding, redirects). The Apprise server endpoint (`apprise_url`) uses netguard `Integration` (LAN/loopback allowed, metadata/link-local blocked). Built on sdk/go/module v0.6.6.

## [0.1.8] - 2026-10-05

### Changed
- Built on core v0.6.14 / sdk/go/module v0.6.4: unregisters on shutdown and re-registers after core restarts (ADR-0022).

## [0.1.7] - 2026-10-05


### Changed
- Reported version comes from muxcore.json (ADR-0021); built on core v0.6.12 / sdk/go/module v0.6.3 (mesh enrollment, ADR-0017).

## [0.1.6] - 2026-10-05

### Changed
- CI runs on GitHub-hosted runners from the umbrella template; retired-origin workflows removed.
- Dependencies resolve from published GitHub tags (no filesystem `replace`); requires core v0.6.0.

## [0.1.5] — 2026-08-10

### Added

- Advertise `settings` capability so admin-ui discovers SettingsProvider without ListAll probing.

## [0.1.4] — 2026-08-10

### Added
- SettingsProvider mesh (`RegisterSettings`) for Apprise URL/token and Discord/Slack/webhook channels.

### Changed
- Pin `core/sdk/go/module` to **v0.5.8**.


## [0.1.3] — 2026-08-10

### Fixed
- Sync Info()/muxcore.json version to **0.1.3**.

## v0.1.0 (2026-08-09)

- Apprise API gateway + optional Discord/Slack/webhook
- Optional MVP host/compose profile (`apprise` / `MVP_ENABLE_NOTIFICATION_APPRISE=1`)
