package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: linkcheck <url>")
		os.Exit(2)
	}

	seed := os.Args[1]
	fmt.Println("seed:", seed)
}
