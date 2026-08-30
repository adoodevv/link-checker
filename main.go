package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/adoodevv/link-checker/internal/extract"
	"github.com/adoodevv/link-checker/internal/fetch"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: linkcheck <url>")
		os.Exit(2)
	}

	seed := os.Args[1]

	client := fetch.New(10 * time.Second)

	res, err := client.Get(context.Background(), seed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fetch failed: %v\n", err)
		os.Exit(1)
	}

	if !res.IsHTML {
		fmt.Printf("%s → %d (%s, not HTML)\n", res.URL, res.StatusCode, res.ContentType)
		return
	}

	parsed, err := extract.Parse(res.URL, res.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("fetched: %s → %d\n", res.URL, res.StatusCode)
	fmt.Printf("base:    %s\n\n", parsed.Base)

	fmt.Printf("links (%d):\n", len(parsed.Links))
	for _, l := range parsed.Links {
		fmt.Printf("  %s\n", l.Resolved)
		if l.Identity != l.Resolved {
			fmt.Printf("      identity: %s\n", l.Identity)
		}
	}

	fmt.Printf("\nskipped (%d):\n", len(parsed.Skipped))
	for _, s := range parsed.Skipped {
		fmt.Printf("  %s  (%s)\n", s.Raw, s.Reason)
	}
}
