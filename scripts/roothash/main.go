package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-keel/keel/pkg/pwd"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: roothash <password>")
		os.Exit(1)
	}
	if strings.TrimSpace(os.Args[1]) == "" {
		fmt.Fprintln(os.Stderr, "error: password must not be empty")
		os.Exit(1)
	}
	h, err := pwd.HashPassword(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(h)
}
