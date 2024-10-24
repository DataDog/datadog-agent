// Package main runs aferofs analyzer
package main

import (
	"log"

	"github.com/DataDog/datadog-agent/pkg/linters/aferofs"
	"golang.org/x/tools/go/analysis/multichecker"
)

func main() {
	p, err := aferofs.New(nil)
	if err != nil {
		log.Fatal(err)
	}
	as, err := p.BuildAnalyzers()
	if err != nil {
		log.Fatal(err)
	}

	multichecker.Main(as...)
}
