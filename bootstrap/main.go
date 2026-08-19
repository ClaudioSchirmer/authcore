// Package main is the composition root of the authcore service.
//
// It holds composition only — no routes, no schemas, no business code.
// Every capability is declared in wire.go as a bootstrap.Feature.
package main

import (
	"log"

	"github.com/ClaudioSchirmer/omnicore/bootstrap"
)

func main() {
	if err := bootstrap.Run(Wire); err != nil {
		log.Fatal(err)
	}
}
