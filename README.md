# linkcheck

A concurrent link checker written in Go. Crawls a site you own, follows internal links,
verifies every link resolves, and reports the broken ones. Built to run in CI: JSON
output and a non-zero exit code when links are broken.

The interesting parts are URL identity, crawl termination, and fragment checking — not
the HTTP plumbing.

## Status

**Phase 1 complete.** Fetches one page, extracts every link, resolves it to an absolute
URL, and computes a canonical identity. Still sequential — concurrency is Phase 2.

## Usage

```bash
go run . https://example.com
```

```
fetched: https://example.com → 200
base:    https://example.com

links (12):
  https://example.com/about
  https://example.com/docs
      identity: https://example.com/docs

skipped (2):
  mailto:hi@example.com  (scheme mailto)
  tel:+233247632002      (scheme tel)
```

Build a binary:

```bash
go build -o linkcheck .
./linkcheck https://example.com
```

### Exit codes

| Code | Meaning |
| ------ | --------- |
| 0 | Ran successfully |
| 1 | Ran, but the fetch failed |
| 2 | Called incorrectly (bad arguments, unparseable URL) |

Phase 4 will reserve `1` for "ran fine, found broken links", which is what CI keys on.

## Development

```bash
go test -race ./...
go vet ./...
```

`-race` enables Go's race detector: it instruments memory accesses and reports
unsynchronised concurrent reads and writes. Phase 1 is sequential so it has nothing to
find — it runs from the start so the habit is in place before goroutines appear.

## URL identity

The crawler constantly asks *have I seen this page before?* Answering it needs a
canonical key, because all of these are the same page:

```
https://example.com/about
https://example.com/about/
https://EXAMPLE.com/about
https://example.com:443/about
https://example.com/about#team
```

Get identity too loose and the crawler loops. Too strict and it silently skips pages,
reporting a site clean when it is not.

**Identity is a comparison key, not a request URL.** If a page links to `/docs/`, we
fetch `/docs/` exactly as written — servers may treat the trailing slash as significant.
Identity is only used for the visited set.

| Rule | Behaviour | Why |
| ------ | ----------- | ----- |
| Scheme | Lowercased | Case-insensitive per RFC 3986 |
| Host | Lowercased | DNS is case-insensitive |
| Path | Case preserved | Many servers treat `/About` and `/about` as different resources |
| Default ports | `:80` and `:443` stripped | `https://ex.com:443/` is `https://ex.com/` |
| Trailing slash | Stripped, except root `/` | Same page on the sites this tool targets |
| Fragment | Stripped from identity, kept on the link | Same document, different anchor. Phase 5 needs it |
| Query | Kept as-is, unsorted | `?id=1` and `?id=2` are different pages |
| Schemes fetched | `http` and `https` only | `mailto:`, `tel:`, `javascript:`, `data:` are skipped, not treated as broken |

The trailing-slash rule is the most arguable one here. It is wrong in the general case —
a server may legitimately serve different content for `/about` and `/about/` — but it is
right for the static sites this tool is built to check.

### Relative resolution

Resolution follows RFC 3986 via `url.URL.ResolveReference`, which has one behaviour that
surprises people:

```
base https://ex.com/docs   + href "intro"  →  https://ex.com/intro
base https://ex.com/docs/  + href "intro"  →  https://ex.com/docs/intro
```

The last path segment is *replaced*, not appended, because `/docs` names a file rather
than a directory. A `<base href>` pointing at a directory must end with a slash or every
relative link on the page silently resolves one level too high.

## Architecture (Phase 1)

```
              seed URL
                 │
                 ▼
            ┌─────────┐
            │ main.go │
            └────┬────┘
                 │
     ┌───────────┼───────────┐
     ▼           ▼           ▼
   urlx        fetch      extract
 identity    GET with    parse HTML
 + fragment  timeout     resolve hrefs
```

- `internal/urlx` — canonical identity for a URL. The whole crawl depends on this.
- `internal/fetch` — one GET with an explicit timeout, a real User-Agent, and a capped
  body read. Grows a tuned `Transport`, retries, rate limiting, and robots.txt in Phase 3.
- `internal/extract` — parses HTML, honours `<base href>`, resolves relative URLs, and
  records skipped links with a reason rather than dropping them.

### Error contract

A 404 is **not** an error. The request succeeded and the server answered "no such page" —
which is precisely what a link checker exists to collect. An error means the request
itself failed: DNS did not resolve, the TLS handshake failed, the timeout elapsed.

Conflating "the server said no" with "I could not reach the server" would make the whole
tool wrong.

## Architecture (planned)

```
         ┌─────────────┐
         │ coordinator │  owns visited set + unbounded frontier
         └──────┬──────┘
                │
   ┌────────────┼────────────┐
   ▼            ▼            ▼
 worker       worker       worker
fetch+parse  fetch+parse  fetch+parse
   │            │            │
   └────────────┼────────────┘
                ▼
        discovered URLs ──┐
                ▲         │
                └─────────┘
```

## The termination problem

*Placeholder — written after Phase 2 lands.*

This section will explain why "stop when the queue is empty" is wrong, why a bounded
channel frontier deadlocks, and how a pending-work counter owned by the coordinator is
what actually terminates the crawl.

## Roadmap

| Phase | Scope |
| ------- | ------- |
| 1 ✅ | Sequential: URL identity, fetch, extract |
| 2 | Coordinator, worker pool, termination, context cancellation |
| 3 | Tuned transport, rate limiting, robots.txt, retries, internal vs external |
| 4 | JSON output, exit codes, failure classification, GitHub Action |
| 5 | Fragment (anchor) checking with a document cache |

## License

MIT. See [LICENSE](LICENSE).
