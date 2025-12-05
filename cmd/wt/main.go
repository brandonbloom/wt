package main

import (
	"log"

	"github.com/brandonbloom/wt/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		log.Fatal(err)
	}
}
