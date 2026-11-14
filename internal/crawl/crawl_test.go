package crawl

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"errors"

	"github.com/adoodevv/link-checker/internal/fetch"
)

func htmlPage(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, body)
	}
}

func mustRun(t *testing.T, seed string, workers int) []PageResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	results, err := Run(ctx, fetch.New(time.Second), seed, workers)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	return results
}

func TestRun_TerminatesOnCycle(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/a", htmlPage(`<a href="/b">b</a>`))
	mux.HandleFunc("/b", htmlPage(`<a href="/c">c</a>`))
	// /c links back to /a (a cycle) and to /b/ (same identity as /b).
	mux.HandleFunc("/c", htmlPage(`<a href="/a">a</a> <a href="/b/">b</a>`))

	srv := httptest.NewServer(mux)
	defer srv.Close()

	for _, n := range []int{1, 4, 16} {
		t.Run(fmt.Sprintf("%d workers", n), func(t *testing.T) {
			results := mustRun(t, srv.URL+"/a", n)

			if len(results) != 3 {
				t.Fatalf("got %d results, want 3: %+v", len(results), results)
			}
			seen := map[string]int{}
			for _, r := range results {
				seen[r.URL]++
			}
			for u, count := range seen {
				if count != 1 {
					t.Errorf("%s visited %d times, want 1", u, count)
				}
			}
		})
	}
}

func TestRun_DoesNotFollowExternalLinks(t *testing.T) {
	ext := http.NewServeMux()
	ext.HandleFunc("/x", htmlPage(`<a href="/y">y</a>`))
	ext.HandleFunc("/y", htmlPage(`should never be fetched`))
	extSrv := httptest.NewServer(ext)
	defer extSrv.Close()

	site := http.NewServeMux()
	site.HandleFunc("/", htmlPage(`<a href="`+extSrv.URL+`/x">external</a>`))
	siteSrv := httptest.NewServer(site)
	defer siteSrv.Close()

	results := mustRun(t, siteSrv.URL+"/", 4)

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2 (seed + one external check): %+v", len(results), results)
	}
	for _, r := range results {
		if strings.HasSuffix(r.URL, "/y") {
			t.Errorf("followed a link on an external page: %s", r.URL)
		}
		if strings.HasSuffix(r.URL, "/x") && r.Internal {
			t.Errorf("%s marked internal, want external", r.URL)
		}
	}
}

func TestRun_StopsOnCancel(t *testing.T) {
	// A page that never answers until the request is cancelled.
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)

	done := make(chan error, 1)
	go func() {
		_, err := Run(ctx, fetch.New(10*time.Second), srv.URL+"/", 4)
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got error %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return within 2s of cancellation")
	}
}
