package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// maxSocialLinks keeps the footer to a row of icons rather than a second
	// wall of buttons: four profiles is as many as the pill can sit beside.
	maxSocialLinks = 4
	// maxSocialLinkLength is generous for a profile URL and still small enough
	// that the setting cannot be used to store something else.
	maxSocialLinkLength = 2048
)

// normalizeSocialLinks reads what the settings API was handed and returns the
// text that is actually stored: a JSON array of https profile URLs, or "" when
// there are none. The empty string is the "off" value, the same way an empty
// footer link means the footer falls back to its default.
func normalizeSocialLinks(value any) (string, error) {
	if value == nil {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("social links must be a JSON array of URLs")
	}
	if strings.TrimSpace(text) == "" {
		return "", nil
	}
	var raw []string
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return "", fmt.Errorf("social links must be a JSON array of URLs")
	}
	if len(raw) > maxSocialLinks {
		return "", fmt.Errorf("use at most %d social links", maxSocialLinks)
	}
	links := make([]string, 0, len(raw))
	seen := make(map[string]bool, len(raw))
	for _, link := range raw {
		normalized, err := normalizeSocialLink(link)
		if err != nil {
			return "", err
		}
		identity := socialLinkIdentity(normalized)
		if seen[identity] {
			return "", fmt.Errorf("duplicate social link %s", normalized)
		}
		seen[identity] = true
		links = append(links, normalized)
	}
	if len(links) == 0 {
		return "", nil
	}
	encoded, err := json.Marshal(links)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// normalizeSocialLink checks one address. The link is shown to every visitor
// as a clickable icon, so it has to be a plain https address to a real host:
// no credentials in front of the hostname, no spaces around it, nothing that a
// browser would read differently from what the administrator pasted.
func normalizeSocialLink(link string) (string, error) {
	if link != strings.TrimSpace(link) {
		return "", fmt.Errorf("social links cannot start or end with a space")
	}
	if link == "" {
		return "", fmt.Errorf("a social link cannot be empty")
	}
	if utf8.RuneCountInString(link) > maxSocialLinkLength {
		return "", fmt.Errorf("a social link cannot be longer than %d characters", maxSocialLinkLength)
	}
	for _, r := range link {
		if r < 32 || r == 127 {
			return "", fmt.Errorf("a social link cannot contain control characters")
		}
	}
	parsed, err := url.Parse(link)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return "", fmt.Errorf("social link %q must be an https URL", link)
	}
	if !validSocialLinkHost(parsed.Hostname()) {
		return "", fmt.Errorf("social link %q must point at a public hostname", link)
	}
	return link, nil
}

// validSocialLinkHost rejects what is not a hostname somebody would publish:
// a bare name, an IP address, or a top label that says the address is a
// placeholder.
func validSocialLinkHost(host string) bool {
	host = strings.ToLower(host)
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 {
			return false
		}
		for _, r := range label {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' {
				return false
			}
		}
	}
	top := labels[len(labels)-1]
	if top == "local" || top == "internal" || top == "invalid" || top == "localhost" {
		return false
	}
	// A top label made only of digits is an IPv4 address, not a site.
	for _, r := range top {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// socialLinkIdentity is what two links are compared by: the same profile with
// or without a trailing slash is the same link, and the scheme and host are
// case-insensitive.
func socialLinkIdentity(link string) string {
	parsed, err := url.Parse(link)
	if err != nil {
		return link
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host + strings.TrimRight(parsed.Path, "/"))
}
