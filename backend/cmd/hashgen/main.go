package main

import (
	"fmt"
	"os"

	"github.com/dsteiman/tickets-general/backend/internal/auth"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: hashgen <password>")
		os.Exit(2)
	}
	h, err := auth.HashPassword(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "hash: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(h)
}
