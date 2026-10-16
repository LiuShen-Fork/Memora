package main

import (
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Unlocking an album is the one place a visitor can guess a secret without an
// account, so it is also the one place that has to count attempts.
//
// Two windows are kept. The per-client one stops a single visitor working
// through a password, and the per-album one bounds the total rate against an
// album however many addresses the attempts come from — which is what still
// holds when the source address cannot be believed.
const (
	albumUnlockWindow    = time.Minute
	albumUnlockPerClient = 8
	albumUnlockPerAlbum  = 32
	// Entries expire after one window, but a flood of distinct addresses could
	// still create many of them within it, so the client map is capped too.
	albumUnlockClientMax = 5000
)

type albumUnlockAttempts struct {
	count   int
	resetAt time.Time
}

var albumUnlockLimits = struct {
	sync.Mutex
	clients map[string]albumUnlockAttempts
	albums  map[int64]albumUnlockAttempts
}{
	clients: map[string]albumUnlockAttempts{},
	albums:  map[int64]albumUnlockAttempts{},
}

// albumUnlockClientKey folds the source address down to the part worth counting.
// Addresses inside one IPv6 /64 belong to the same network and are cheap to
// rotate, so counting them separately would count nothing; an IPv4-mapped
// address is the IPv4 address it says it is.
func albumUnlockClientKey(raw string) string {
	host := strings.TrimSpace(raw)
	if host == "" {
		return "unknown"
	}
	if zone := strings.IndexByte(host, '%'); zone != -1 {
		host = host[:zone]
	}
	host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	ip := net.ParseIP(host)
	if ip == nil {
		return "u:" + strings.ToLower(host)
	}
	if v4 := ip.To4(); v4 != nil {
		return "4:" + v4.String()
	}
	return "6:" + ip.Mask(net.CIDRMask(64, 128)).String()
}

// albumUnlockAllowed records an attempt and reports whether it may go ahead.
// The attempt is counted before the password is checked, so a blocked album
// stays blocked for the rest of the window instead of being reset by a guess
// that happens to be right.
func albumUnlockAllowed(albumID int64, client string) bool {
	key := strconv.FormatInt(albumID, 10) + ":" + albumUnlockClientKey(client)
	now := time.Now()

	albumUnlockLimits.Lock()
	defer albumUnlockLimits.Unlock()
	pruneUnlockLimits(now)

	if blocked(albumUnlockLimits.clients[key], now, albumUnlockPerClient) ||
		blocked(albumUnlockLimits.albums[albumID], now, albumUnlockPerAlbum) {
		return false
	}
	albumUnlockLimits.clients[key] = advanceUnlockAttempt(albumUnlockLimits.clients[key], now)
	albumUnlockLimits.albums[albumID] = advanceUnlockAttempt(albumUnlockLimits.albums[albumID], now)
	return true
}

// albumUnlockSucceeded forgives the visitor's own attempts, so someone who
// mistyped their password a few times and then got it right starts clean. The
// per-album window is left alone: it exists to bound the guessing, and a
// correct guess does not undo the guessing that came before it.
func albumUnlockSucceeded(albumID int64, client string) {
	key := strconv.FormatInt(albumID, 10) + ":" + albumUnlockClientKey(client)
	albumUnlockLimits.Lock()
	defer albumUnlockLimits.Unlock()
	delete(albumUnlockLimits.clients, key)
}

func blocked(attempt albumUnlockAttempts, now time.Time, limit int) bool {
	return attempt.resetAt.After(now) && attempt.count >= limit
}

func advanceUnlockAttempt(attempt albumUnlockAttempts, now time.Time) albumUnlockAttempts {
	if !attempt.resetAt.After(now) {
		return albumUnlockAttempts{count: 1, resetAt: now.Add(albumUnlockWindow)}
	}
	attempt.count++
	return attempt
}

func pruneUnlockLimits(now time.Time) {
	for key, attempt := range albumUnlockLimits.clients {
		if !attempt.resetAt.After(now) {
			delete(albumUnlockLimits.clients, key)
		}
	}
	for id, attempt := range albumUnlockLimits.albums {
		if !attempt.resetAt.After(now) {
			delete(albumUnlockLimits.albums, id)
		}
	}
	// Dropping every client entry only ever forgives attempts, never invents
	// them; the per-album window is what keeps the bound.
	if len(albumUnlockLimits.clients) >= albumUnlockClientMax {
		albumUnlockLimits.clients = map[string]albumUnlockAttempts{}
	}
}
