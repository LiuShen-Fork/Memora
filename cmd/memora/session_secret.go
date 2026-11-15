package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const (
	// sessionSecretFileName holds the key the session cookie is signed with
	// when the environment does not supply one. It lives beside the database so
	// backing up the data directory backs up the secret with it.
	sessionSecretFileName = ".session-secret"
	// minSessionSecretLength matches the length upstream asks for. A shorter
	// value still works, but it is worth saying so in the log.
	minSessionSecretLength = 32
	// sessionSecretSize is how many random bytes are generated, the same
	// amount upstream writes.
	sessionSecretSize = 48
)

// errSessionSecretExists reports that another process created the secret file
// between the read and the write.
var errSessionSecretExists = errors.New("session secret file already exists")

type sessionSecretEnv struct {
	name string
	// deprecated names stay readable so an existing deployment survives the
	// rename, but they are no longer advertised.
	deprecated bool
}

// sessionSecretEnvNames are tried in order. The upstream spelling is kept in
// the middle so a ChronoFrame deployment moves over without an edit.
var sessionSecretEnvNames = []sessionSecretEnv{
	{name: "MEMORA_SESSION_SECRET"},
	{name: "NUXT_SESSION_PASSWORD"},
	{name: "CFRAME_SESSION_SECRET", deprecated: true},
}

// resolveSessionSecret returns the key the session cookie is signed with.
//
// An environment variable wins; otherwise the key stored in data/.session-secret
// is reused; otherwise a new one is generated and stored. Nothing is generated
// while an existing key is in place, so restarts keep clients signed in, and an
// install that always set the variable never reads or writes the file.
func resolveSessionSecret(dataDir string, getenv func(string) string) ([]byte, error) {
	for _, candidate := range sessionSecretEnvNames {
		value := strings.TrimSpace(getenv(candidate.name))
		if value == "" {
			continue
		}
		if candidate.deprecated {
			log.Printf("%s is deprecated; rename the variable to %s", candidate.name, sessionSecretEnvNames[0].name)
		}
		if len(value) < minSessionSecretLength {
			log.Printf("%s is shorter than %d characters; use a longer random value", candidate.name, minSessionSecretLength)
		}
		return []byte(value), nil
	}

	path := filepath.Join(dataDir, sessionSecretFileName)
	stored, exists, usable, err := readSessionSecret(path)
	if err != nil {
		return nil, err
	}
	if usable {
		return stored, nil
	}
	if exists {
		// A file too short to be a key cannot sign a session safely. Replace it
		// rather than trusting it.
		log.Printf("%s cannot sign sessions; replacing it", path)
	}

	generated, err := generateSessionSecret()
	if err != nil {
		return nil, err
	}
	if err := writeSessionSecret(path, generated, exists); err != nil {
		if !errors.Is(err, errSessionSecretExists) {
			return nil, err
		}
		// Another process created the file first. Its key is the one to use.
		stored, _, usable, err := readSessionSecret(path)
		if err != nil {
			return nil, err
		}
		if !usable {
			return nil, fmt.Errorf("%s was created by another process but cannot sign sessions", path)
		}
		return stored, nil
	}
	log.Printf("generated a session secret and saved it to %s; back it up with the data directory", path)
	return []byte(generated), nil
}

// readSessionSecret reports what the stored file holds: whether it exists, and
// whether it is long enough to sign sessions.
func readSessionSecret(path string) (stored []byte, exists bool, usable bool, err error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, false, nil
	}
	if err != nil {
		return nil, false, false, err
	}
	value := strings.TrimSpace(string(raw))
	if len(value) < minSessionSecretLength {
		return nil, true, false, nil
	}
	return []byte(value), true, true, nil
}

func generateSessionSecret() (string, error) {
	buf := make([]byte, sessionSecretSize)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// writeSessionSecret stores the key. Unless replace is set it refuses to
// overwrite a file that appeared in the meantime.
func writeSessionSecret(path, secret string, replace bool) error {
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if replace {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	file, err := os.OpenFile(path, flags, 0o600)
	if errors.Is(err, os.ErrExist) {
		return errSessionSecretExists
	}
	if err != nil {
		return err
	}
	if _, err := file.WriteString(secret); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}
