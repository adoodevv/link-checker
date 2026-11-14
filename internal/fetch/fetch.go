// Package fetch performs single HTTP requests for the crawler.
package fetch

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"
)

// UserAgent identifies this crawler to the servers it visits.
const UserAgent = "linkcheck/0.1 (+https://github.com/adoodevv/link-checker)"

// maxBodyBytes caps how much of a response we will read into memory.
const maxBodyBytes = 10 << 20 // 10 MiB

// Result is the outcome of fetching one URL.
type Result struct {
	URL         string
	StatusCode  int
	ContentType string
	Body        []byte
	IsHTML      bool
}

// Client fetches URLs over HTTP.
type Client struct {
	httpClient *http.Client
}

// New returns a Client whose requests time out after the given duration.
func New(timeout time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
	}
}

// Get retrieves rawURL. It returns an error only when the request itself
// failed; an HTTP error status such as 404 is a successful fetch with a
// non-2xx StatusCode.
func (c *Client) Get(ctx context.Context, rawURL string) (*Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err // *url.Error already names the method and URL
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	isHTML := false
	if mediaType, _, err := mime.ParseMediaType(ct); err == nil {
		isHTML = strings.EqualFold(mediaType, "text/html") ||
			strings.EqualFold(mediaType, "application/xhtml+xml")
	}

	var body []byte
	if isHTML {
		body, err = io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
		if err != nil {
			return nil, fmt.Errorf("read body of %q: %w", rawURL, err)
		}
	} else {
		// Drain non-HTML so the connection can be reused, but don't keep it.
		io.Copy(io.Discard, io.LimitReader(resp.Body, maxBodyBytes))
	}

	return &Result{
		URL:         rawURL,
		StatusCode:  resp.StatusCode,
		ContentType: ct,
		Body:        body,
		IsHTML:      isHTML,
	}, nil
}
