// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// CLI for generating SymDB data from binaries.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb"
)

var (
	pprofPort  = flag.Int("pprof-port", 8081, "Port for pprof server.")
	binaryPath = flag.String("binary-path", "", "Path to the binary to analyze.")
)

func main() {
	flag.Parse()
	if *binaryPath == "" {
		fmt.Println("Usage: symdbcli --binary-path <path-to-binary> [--pprof-port <port>]")
		os.Exit(1)
	}

	// Start the pprof server.
	go func() {
		_ = http.ListenAndServe(fmt.Sprintf("localhost:%d", *pprofPort), nil)
	}()

	if err := run(*binaryPath); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func run(binaryPath string) error {
	log.Printf("Analyzing binary: %s", binaryPath)

	file, err := object.OpenElfFile(binaryPath)
	if err != nil {
		return err
	}
	symBuilder, err := symdb.NewSymDBBuilder(file)
	if err != nil {
		return err
	}
	symbols, err := symBuilder.ExtractSymbols()
	if err != nil {
		return err
	}
	stats := statsFromSymbols(symbols)
	log.Printf("Symbol statistics for %s: %+v", binaryPath, stats)
	return nil
}

type symbolStats struct {
	numPackages    int
	numTypes       int
	numFunctions   int
	numSourceFiles int
}

func statsFromSymbols(s symdb.Symbols) symbolStats {
	stats := symbolStats{
		numPackages:    len(s.Packages),
		numTypes:       0,
		numFunctions:   0,
		numSourceFiles: 0,
	}
	sourceFiles := make(map[string]struct{})
	for _, pkg := range s.Packages {
		stats.numTypes += len(pkg.Types)
		stats.numFunctions += len(pkg.Functions)
		for _, f := range pkg.Functions {
			_, ok := sourceFiles[f.File]
			if !ok {
				sourceFiles[f.File] = struct{}{}
				stats.numSourceFiles++
			}
		}
	}
	return stats
}
