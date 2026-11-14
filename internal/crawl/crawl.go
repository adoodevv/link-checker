// Package crawl coordinates concurrent fetching of a site, tracking which
// pages have been visited and when the crawl is complete.
package crawl

import (
	"context"
	"fmt"
	"net/url"
	"sync"

	"github.com/adoodevv/link-checker/internal/extract"
	"github.com/adoodevv/link-checker/internal/fetch"
	"github.com/adoodevv/link-checker/internal/urlx"
)

// PageResult is what one worker reports back after fetching a URL.
type PageResult struct {
	URL        string
	Internal   bool // true if on the seed's host, so it was crawled
	StatusCode int
	Err        error
	Links      []extract.Link
}

// job is one unit of work handed to a worker.
type job struct {
	url      string
	internal bool
}

// Run crawls every page on seed's host using n workers. Links to other
// hosts are fetched once to check they resolve, but never followed.
func Run(ctx context.Context, client *fetch.Client, seed string, n int) ([]PageResult, error) {
	seedID, err := urlx.Identity(seed)
	if err != nil {
		return nil, fmt.Errorf("seed URL: %w", err)
	}
	seedHost, err := hostOf(seedID)
	if err != nil {
		return nil, fmt.Errorf("seed host: %w", err)
	}

	jobs := make(chan job)
	reports := make(chan PageResult)

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Go(func() {
			worker(ctx, client, jobs, reports)
		})
	}

	// Only this goroutine ever touches these three. No mutex needed.
	visited := map[string]bool{seedID: true}
	frontier := []job{{url: seed, internal: true}}
	pending := 0

	var results []PageResult

	for {
		// A nil channel is never selected. When there's nothing to hand
		// out, sendCh stays nil and the send case drops out of the select.
		var sendCh chan job
		var next job
		if len(frontier) > 0 {
			sendCh = jobs
			next = frontier[0]
		}

		select {
		case sendCh <- next:
			frontier = frontier[1:]
			pending++

		case r := <-reports:
			pending--
			results = append(results, r)

			for _, l := range r.Links {
				if visited[l.Identity] {
					continue
				}
				visited[l.Identity] = true

				h, err := hostOf(l.Identity)
				if err != nil {
					continue
				}
				frontier = append(frontier, job{url: l.Resolved, internal: h == seedHost})
			}

			if pending == 0 && len(frontier) == 0 {
				close(jobs)
				wg.Wait()
				return results, nil
			}

		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return results, ctx.Err()
		}
	}
}

// hostOf returns the host of an identity. Identities are already
// normalised, so the host is lowercased with default ports stripped.
func hostOf(id string) (string, error) {
	u, err := url.Parse(id)
	if err != nil {
		return "", err
	}
	return u.Host, nil
}

func worker(ctx context.Context, client *fetch.Client, jobs <-chan job, reports chan<- PageResult) {
	for j := range jobs {
		pr := check(ctx, client, j)

		select {
		case reports <- pr:
		case <-ctx.Done():
			return // nobody is listening anymore; stop
		}
	}
}

func check(ctx context.Context, client *fetch.Client, j job) PageResult {
	pr := PageResult{URL: j.url, Internal: j.internal}

	res, err := client.Get(ctx, j.url)
	if err != nil {
		pr.Err = err
		return pr
	}
	pr.StatusCode = res.StatusCode

	if j.internal && res.IsHTML && res.StatusCode < 400 {
		if parsed, err := extract.Parse(res.URL, res.Body); err == nil {
			pr.Links = parsed.Links
		}
	}
	return pr
}
