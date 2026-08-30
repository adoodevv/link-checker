// Package extract pulls links out of an HTML document and resolves them
// against the page they were found on.
package extract

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"

	"github.com/adoodevv/link-checker/internal/urlx"
)

// Link is one href found on a page, resolved to an absolute URL.
type Link struct {
	Raw      string // href exactly as written in the document
	Resolved string // absolute URL to request
	Identity string // canonical key for the visited set
	Fragment string // anchor without "#", empty if none
}

// Skipped is an href we deliberately did not follow.
type Skipped struct {
	Raw    string
	Reason string
}

// Result holds everything found on one page.
type Result struct {
	Base    string
	Links   []Link
	Skipped []Skipped
}

// skippableSchemes are non-HTTP schemes we never fetch.
var skippableSchemes = map[string]bool{
	"mailto": true, "tel": true, "javascript": true,
	"data": true, "ftp": true, "sms": true, "file": true,
}

// Parse finds every <a href> in body and resolves it against pageURL,
// honouring the first <base href> in document order.
func Parse(pageURL string, body []byte) (*Result, error) {
	pageU, err := url.Parse(pageURL)
	if err != nil {
		return nil, fmt.Errorf("parse page URL %q: %w", pageURL, err)
	}

	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("parse HTML of %q: %w", pageURL, err)
	}

	base := pageU
	if href, ok := findBase(doc); ok {
		if b, err := pageU.Parse(href); err == nil {
			base = b
		}
	}

	res := &Result{Base: base.String()}

	for _, raw := range findHrefs(doc) {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			res.Skipped = append(res.Skipped, Skipped{raw, "empty href"})
			continue
		}

		ref, err := url.Parse(trimmed)
		if err != nil {
			res.Skipped = append(res.Skipped, Skipped{raw, "unparseable"})
			continue
		}

		if s := strings.ToLower(ref.Scheme); s != "" && skippableSchemes[s] {
			res.Skipped = append(res.Skipped, Skipped{raw, "scheme " + s})
			continue
		}

		abs := base.ResolveReference(ref)
		if s := strings.ToLower(abs.Scheme); s != "http" && s != "https" {
			res.Skipped = append(res.Skipped, Skipped{raw, "scheme " + s})
			continue
		}

		id, err := urlx.Identity(abs.String())
		if err != nil {
			res.Skipped = append(res.Skipped, Skipped{raw, "no identity"})
			continue
		}

		res.Links = append(res.Links, Link{
			Raw:      raw,
			Resolved: abs.String(),
			Identity: id,
			Fragment: abs.Fragment,
		})
	}

	return res, nil
}

// findBase returns the href of the first <base> element in document order.
func findBase(n *html.Node) (string, bool) {
	if n.Type == html.ElementNode && n.Data == "base" {
		for _, a := range n.Attr {
			if strings.EqualFold(a.Key, "href") {
				return a.Val, true
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if href, ok := findBase(c); ok {
			return href, true
		}
	}
	return "", false
}

// findHrefs returns every <a href> value in document order.
func findHrefs(n *html.Node) []string {
	var out []string
	if n.Type == html.ElementNode && n.Data == "a" {
		for _, a := range n.Attr {
			if strings.EqualFold(a.Key, "href") {
				out = append(out, a.Val)
				break
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		out = append(out, findHrefs(c)...)
	}
	return out
}