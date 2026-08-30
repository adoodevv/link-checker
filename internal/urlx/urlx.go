// Package urlx computes canonical identities for URLs so a crawler can
// tell whether it has already visited a page.
package urlx

import (
	"fmt"
	"net/url"
	"strings"
)

// Identity returns the canonical comparison key for raw.
//
// The result is NOT necessarily a URL you should request. Identity strips
// trailing slashes and fragments to decide sameness; fetch the URL as the
// page actually wrote it.
func Identity(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse %q: %w", raw, err)
	}

	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""

	if (u.Scheme == "http" && u.Port() == "80") ||
		(u.Scheme == "https" && u.Port() == "443") {
		u.Host = u.Hostname()
	}

	if u.Path == "" {
		u.Path = "/"
	} else if len(u.Path) > 1 {
		u.Path = strings.TrimSuffix(u.Path, "/")
	}

	return u.String(), nil
}