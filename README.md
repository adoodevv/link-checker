# linkcheck

A concurrent link checker written in Go. Crawls a site you own, follows internal links,
verifies every link resolves, and reports the broken ones. Built to run in CI: JSON
output and a non-zero exit code when links are broken.

The interesting parts are URL identity, crawl termination, and fragment checking — not
the HTTP plumbing.

## Status

**Phase 2 complete.** Crawls a whole site concurrently with a pool of workers, follows
internal links, checks external links once without following them, terminates on its
own, and shuts down cleanly on Ctrl-C with partial results.

Not yet: rate limiting, robots.txt, retries (Phase 3), JSON output and CI exit codes
(Phase 4), anchor checking (Phase 5).

## Usage

```bash
go build -o linkcheck .
./linkcheck https://mikrobotacademy.com
```

```
  int 200  https://mikrobotacademy.com
  int 200  https://mikrobotacademy.com/programs
  int 200  https://mikrobotacademy.com/shop/feetech-sts3215-servo
  ext 200  https://robofest.net/
  ext 404  https://www.youtube.com/@mikrobotacademy
  ext ERR  https://www.facebook.com/mikrobotacademy
           Get "https://www.facebook.com/mikrobotacademy": context deadline exceeded
  ...

18 internal, 22 external, 3 broken
```

`int` pages are on the seed's host: they are fetched, parsed, and their links followed.
`ext` links point elsewhere: they are fetched once to check they resolve, never parsed.

Results print in the order workers finish, so the order changes between runs.

Press Ctrl-C at any point to stop. The crawl winds down, prints everything checked so
far, and reports that the results are partial.

### Exit codes

| Code | Meaning |
| ------ | --------- |
| 0 | Crawl finished |
| 1 | Crawl could not run (for example, the seed URL could not be parsed) |
| 2 | Called incorrectly (wrong number of arguments) |
| 130 | Interrupted with Ctrl-C; results printed are partial |

Broken links do not change the exit code yet. Phase 4 makes `1` mean "ran fine, found
broken links", which is what CI keys on.

## Development

```bash
go test -race -timeout 20s ./...
go vet ./...
```

`-race` enables Go's race detector, which reports unsynchronised concurrent reads and
writes of the same memory. The crawl tests run the coordinator with 1, 4, and 16 workers
under it.

`-timeout` turns a hang into a failure. A termination bug in a crawler doesn't crash —
it waits forever — so every crawl test also runs under its own context deadline.

## Architecture

```
                    seed URL
                       │
                       ▼
              ┌─────────────────┐
              │   coordinator   │  owns: visited set, frontier, pending count
              └───┬─────────▲───┘
            jobs  │         │  reports
                  ▼         │
   ┌──────────┬──────────┬──────────┐
   │  worker  │  worker  │  worker  │   fetch → parse (internal only) → report
   └──────────┴──────────┴──────────┘
```

| Package | Role |
| --------- | ------ |
| `internal/urlx` | Canonical identity for a URL. The whole crawl depends on this |
| `internal/fetch` | One GET with an explicit timeout, a real User-Agent, and a capped body read |
| `internal/extract` | Parses HTML, honours `<base href>`, resolves relative URLs, records skipped links with a reason |
| `internal/crawl` | The coordinator and worker pool |

### Error contract

A 404 is **not** an error. The request succeeded and the server answered "no such page",
which is exactly what a link checker exists to collect. An error means the request itself
failed: DNS did not resolve, the TLS handshake failed, the timeout elapsed.

Conflating "the server said no" with "I could not reach the server" would make the whole
tool wrong.

## The termination problem

A crawler's work feeds itself. Workers fetch pages, pages contain links, links become new
work. So the hard question is not how to crawl — it is how to know when you are finished.

### Why "the queue is empty" is wrong

The obvious answer is to stop when there is nothing left in the queue. It is wrong,
because work does not only live in the queue. A worker that has taken a URL and is
halfway through fetching it is holding work the queue cannot see. The queue can be empty
for a moment while every worker is mid-fetch, about to report forty new links. Stopping
then truncates the crawl, and the tool still exits successfully — a silent wrong answer,
the worst kind of bug.

### What actually works: count work in flight

The coordinator keeps one number, `pending`: work that has been handed to a worker and
not yet reported back.

```
hand a URL to a worker   →  pending++
receive a worker report  →  pending--
pending == 0 and nothing left to hand out  →  done
```

This counts work wherever it physically is. It only reaches zero when no worker holds
anything and nothing is waiting to be handed out, so there is genuinely nothing left.

Two details make the count trustworthy:

- **`pending` goes up only when a worker has actually taken the job.** The `jobs`
  channel is unbuffered, so a send completes only when a worker receives it. There is no
  moment where work sits "half handed out" and is not counted.
- **The done check needs both conditions.** `pending == 0` alone could fire while URLs
  are still waiting in the frontier.

### Only the coordinator touches shared state

The visited set, the frontier, and `pending` are read and written by exactly one
goroutine: the coordinator. Workers never see them. They receive jobs on one channel and
send reports on another.

That is why there is no mutex anywhere in the crawl. Each piece of state has one owner,
and everyone else communicates with that owner through channels. Deduplication happens
in the coordinator when links arrive, because only the coordinator knows the full set of
pages already visited.

### Why the frontier is a slice, not a channel

The frontier could have been a buffered channel. That deadlocks: a page with more links
than the buffer holds blocks the worker trying to report them, and if every worker is
blocked sending, nothing ever drains the channel. The frontier is a slice owned by the
coordinator, so it can grow as large as the site needs.

### Internal versus external

Links on the seed's host are crawled: fetched, parsed, followed. Links anywhere else are
checked once and never followed. Without this rule the crawl never terminates, because
following external links means crawling Facebook, YouTube, and everything they link to.
The coordinator decides which is which when it enqueues a link, because it is the only
place that knows the seed's host.

### Stopping cleanly on Ctrl-C

Ctrl-C cancels a context that is passed through the coordinator, every worker, and every
HTTP request. The coordinator stops handing out work, in-flight requests are aborted, and
`Run` returns whatever it collected.

This exposed a goroutine leak. When the coordinator stopped, a worker whose request had
just been cancelled would try to report back — and block forever, because nobody was
listening on the reports channel anymore.

Two changes fixed it:

- **`Run` waits for every worker before returning**, using a `sync.WaitGroup`. A
  function that starts goroutines owns them, and should not return while they are still
  running. Adding this first made the leak visible: the cancellation test hung instead of
  quietly passing.
- **Workers give up on reporting if the context is cancelled.** The send is a `select`
  between delivering the report and `ctx.Done()`, so a worker can never block on a send
  nobody will receive.

A side benefit: requests aborted by Ctrl-C are not reported as broken links. They were
not broken; we stopped checking them.

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

## Known limitations

- **`www` is a different host.** `www.example.com` and `example.com` are treated as
  separate sites, so a site that links to itself with both forms will check one as
  external. Strict is the safe default.
- **Some sites block non-browser clients.** Facebook, for example, often stalls instead
  of answering, which shows up as a timeout. These are not necessarily broken links.
  Phase 4 separates hard failures (404, DNS) from soft ones (timeouts, rate limits).
- **External links are fetched with GET.** HEAD would be cheaper. Phase 3.
- **No politeness yet.** No rate limiting and no robots.txt, so only point it at sites
  you own. Phase 3.

## Roadmap

| Phase | Scope |
| ------- | ------- |
| 1 ✅ | Sequential: URL identity, fetch, extract |
| 2 ✅ | Coordinator, worker pool, termination, internal vs external, clean cancellation |
| 3 | Tuned transport, HEAD with GET fallback, rate limiting, robots.txt, retries |
| 4 | JSON output, sorted results, exit codes, hard vs soft failures, GitHub Action |
| 5 | Fragment (anchor) checking with a document cache |

## License

MIT. See [LICENSE](LICENSE).
