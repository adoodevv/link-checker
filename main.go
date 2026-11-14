package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/adoodevv/link-checker/internal/crawl"
	"github.com/adoodevv/link-checker/internal/fetch"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: linkcheck <url>")
		os.Exit(2)
	}

	seed := os.Args[1]

	client := fetch.New(10 * time.Second)

	// A context that is cancelled when the user presses Ctrl-C.
	// Before, we used context.Background(), which can never be cancelled.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	results, err := crawl.Run(ctx, client, seed, 8)

	// Was the error just "the user pressed Ctrl-C"?
	interrupted := errors.Is(err, context.Canceled)

	// Any other error is a real failure.
	if err != nil && !interrupted {
		fmt.Fprintf(os.Stderr, "crawl error: %v\n", err)
		os.Exit(1)
	}

	// Print results. If interrupted, these are whatever was checked before Ctrl-C.
	var internal, external, broken int
	for _, r := range results {
		kind := "ext"
		if r.Internal {
			kind = "int"
			internal++
		} else {
			external++
		}

		status := fmt.Sprintf("%d", r.StatusCode)
		if r.Err != nil {
			status = "ERR"
			broken++
		} else if r.StatusCode >= 400 {
			broken++
		}

		fmt.Printf("  %-3s %-4s %s\n", kind, status, r.URL)
		if r.Err != nil {
			fmt.Printf("           %v\n", r.Err)
		}
	}
	fmt.Printf("\n%d internal, %d external, %d broken\n", internal, external, broken)

	if interrupted {
		fmt.Fprintln(os.Stderr, "interrupted: results are partial")
		os.Exit(130)
	}
}
