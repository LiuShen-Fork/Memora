# Adding A Setting

Settings are defined in `cmd/chronoframe/settings_defaults.go`.

1. Add a `settingDefault` entry to `defaultSettings` with its namespace, key, type, default value, and visibility.
2. Add matching translation keys under `i18n/locales/en.json` and `i18n/locales/zh-Hans.json`.
3. Add UI-specific behavior in the relevant Vue settings page only when the generic `SettingField` is not enough.
4. Run `go test ./...` and `pnpm lint`.

Do not add a Node/Nitro route or change the existing SQLite table layout. The Go service creates missing default rows at startup and keeps existing values intact.
