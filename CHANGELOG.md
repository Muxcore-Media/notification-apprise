# Changelog

## [0.1.5] — 2026-08-10

### Added

- Advertise `settings` capability so admin-ui discovers SettingsProvider without ListAll probing.

## [0.1.4] — 2026-08-10

### Added
- SettingsProvider mesh (`RegisterSettings`) for Apprise URL/token and Discord/Slack/webhook channels.

### Changed
- Pin `core/sdk/go/module` to **v0.5.2**.


## [0.1.3] — 2026-08-10

### Fixed
- Sync Info()/muxcore.json version to **0.1.3**.

## v0.1.0 (2026-08-09)

- Apprise API gateway + optional Discord/Slack/webhook
- Optional MVP host/compose profile (`apprise` / `MVP_ENABLE_NOTIFICATION_APPRISE=1`)
