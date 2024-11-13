// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
