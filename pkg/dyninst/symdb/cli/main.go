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
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb/symdbutil"
)

var (
	pprofPort      = flag.Int("pprof-port", 8081, "Port for pprof server.")
	binaryPath     = flag.String("binary-path", "", "Path to the binary to analyze.")
	mainModuleOnly = flag.Bool("main-module-only", false, "If set, only symbols from the module containing the \"main\" function are printed.")
	silent         = flag.Bool("silent", false, "If set, the collected symbols are not printed.")
)

func main() {
	flag.Parse()
	if *binaryPath == "" {
		fmt.Print(`Usage: symdbcli --binary-path <path-to-binary> [--main-module-only] [--silent]

The symbols from the specified binary will be extracted and printed to stdout
(unless --silent is specified).
`)
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
	start := time.Now()
	symBuilder, err := symdb.NewSymDBBuilder(binaryPath)
	if err != nil {
		return err
	}
	symbols, err := symBuilder.ExtractSymbols()
	if err != nil {
		return err
	}
	log.Printf("Symbol extraction completed in %s.", time.Since(start))
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
		if *mainModuleOnly && !strings.HasPrefix(pkg.Name, s.MainModule) {
			continue
		}
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

	if !*silent {
		s.Serialize(symdbutil.MakePanickingWriter(os.Stdout), symdb.SerializationOptions{
			OnlyMainModule: *mainModuleOnly,
			PackageSerializationOptions: symdb.PackageSerializationOptions{
				StripLocalFilePrefix: false,
			},
		})
	} else {
		log.Println("--silent specified; symbols not serialized.")
	}

	return stats
}
