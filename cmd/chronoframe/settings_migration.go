package main

func (a *App) migrateEnvironmentSettings() {
	migrations := []struct {
		namespace string
		key       string
		envKey    string
		fallback  string
	}{
		{"app", "title", "NUXT_PUBLIC_APP_TITLE", "ChronoFrame"},
		{"app", "slogan", "NUXT_PUBLIC_APP_SLOGAN", ""},
		{"app", "author", "NUXT_PUBLIC_APP_AUTHOR", ""},
		{"app", "avatarUrl", "NUXT_PUBLIC_APP_AVATAR_URL", ""},
		{"map", "provider", "NUXT_PUBLIC_MAP_PROVIDER", "maplibre"},
		{"map", "mapbox.style", "NUXT_PUBLIC_MAP_MAPBOX_STYLE", ""},
		{"map", "maplibre.style", "NUXT_PUBLIC_MAP_MAPLIBRE_STYLE", ""},
		{"location", "nominatim.baseUrl", "NUXT_NOMINATIM_BASE_URL", ""},
		{"system", "auth.github.clientId", "NUXT_OAUTH_GITHUB_CLIENT_ID", ""},
		{"system", "auth.github.clientSecret", "NUXT_OAUTH_GITHUB_CLIENT_SECRET", ""},
	}
	for _, migration := range migrations {
		value := env(migration.envKey, migration.fallback)
		if value == "" {
			continue
		}
		var current any
		if !a.readSetting(migration.namespace, migration.key, &current) || current == nil || current == migration.fallback {
			a.setSetting(migration.namespace, migration.key, value)
		}
	}
	if envBool("NUXT_PUBLIC_OAUTH_GITHUB_ENABLED", false) {
		var current any
		if !a.readSetting("system", "auth.github.enabled", &current) || current == false {
			a.setSetting("system", "auth.github.enabled", true)
		}
	}
	if token := env("NUXT_MAPBOX_ACCESS_TOKEN", ""); token != "" {
		var current any
		if !a.readSetting("map", "mapbox.token", &current) || current == "" {
			a.setSetting("map", "mapbox.token", token)
		}
	}
	if token := env("NUXT_PUBLIC_MAP_MAPLIBRE_TOKEN", ""); token != "" {
		var current any
		if !a.readSetting("map", "maplibre.token", &current) || current == "" {
			a.setSetting("map", "maplibre.token", token)
		}
	}
}
