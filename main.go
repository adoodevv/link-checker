package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/adoodevv/link-checker/internal/fetch"
	"github.com/adoodevv/link-checker/internal/urlx"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: linkcheck <url>")
		os.Exit(2)
	}

	seed := os.Args[1]

	id, err := urlx.Identity(seed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bad seed URL: %v\n", err)
		os.Exit(2)
	}

	client := fetch.New(10 * time.Second)

	res, err := client.Get(context.Background(), seed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fetch failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("fetched:  %s\n", res.URL)
	fmt.Printf("identity: %s\n", id)
	fmt.Printf("status:   %d\n", res.StatusCode)
	fmt.Printf("type:     %s\n", res.ContentType)
	fmt.Printf("html:     %t\n", res.IsHTML)
	fmt.Printf("bytes:    %d\n", len(res.Body))
}
