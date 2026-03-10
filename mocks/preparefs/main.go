package main

import (
	"fmt"
	"os"

	"github.com/ci4rail/moducop-core-api-server/mocks/mockmender"
)

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "Usage: preparefs")
		os.Exit(2)
	}
	if err := mockmender.PrepareFilesystem(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
