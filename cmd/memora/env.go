package main

import (
	"log"
	"os"
	"strings"
	"sync"
)

// Process settings used to be handed over with a CFRAME_ prefix. They now use
// MEMORA_, and the old spelling keeps working so existing deployments need no
// change, with one warning in the log.
const (
	envPrefix       = "MEMORA_"
	legacyEnvPrefix = "CFRAME_"
)

var (
	deprecatedEnvMu   sync.Mutex
	deprecatedEnvSeen = map[string]bool{}
)

// envValue reads a variable under its current name. A current name is also
// answered by its old spelling, which is reported as deprecated.
func envValue(key string) (string, bool) {
	if rest, renamed := strings.CutPrefix(key, envPrefix); renamed {
		if value, ok := lookupEnv(key); ok {
			return value, true
		}
		legacy := legacyEnvPrefix + rest
		value, ok := lookupEnv(legacy)
		if ok {
			noteDeprecatedEnv(legacy, key)
		}
		return value, ok
	}
	return lookupEnv(key)
}

// lookupEnv reports a variable as set only when it holds something other than
// whitespace, so an empty line in a Compose file does not shadow a default.
func lookupEnv(key string) (string, bool) {
	value := strings.TrimSpace(os.Getenv(key))
	return value, value != ""
}

// noteDeprecatedEnv warns about an old spelling once per process, so a restart
// log stays readable even when several settings still use it.
func noteDeprecatedEnv(old, renamed string) {
	deprecatedEnvMu.Lock()
	seen := deprecatedEnvSeen[old]
	deprecatedEnvSeen[old] = true
	deprecatedEnvMu.Unlock()
	if !seen {
		log.Printf("%s is deprecated; rename the variable to %s", old, renamed)
	}
}
