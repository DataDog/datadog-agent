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
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime/trace"
	"time"

	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb/symdbutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	binaryPath     = flag.String("binary-path", "", "Path to the binary to analyze.")
	silent         = flag.Bool("silent", false, "If set, the collected symbols are not printed.")
	onlyFirstParty = flag.Bool("only-1stparty", false,
		"Only output symbols for \"1st party\" code (i.e. code from modules belonging "+
			"to the same GitHub org as the main one).")

	pprofPort = flag.Int("pprof-port", 8081, "Port for pprof server.")
	traceFile = flag.String("trace", "", "Path to the file to save an execution trace to.")
)

func main() {
	flag.Parse()
	if *binaryPath == "" {
		fmt.Print(`Usage: symdbcli --binary-path <path-to-binary> [--only-1stparty] [--silent]

The symbols from the specified binary will be extracted and printed to stdout
(unless --silent is specified).
`)
		os.Exit(1)
	}

	logLevel := os.Getenv("DD_LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	log.SetupLogger(log.Default(), logLevel)
	defer log.Flush()

	// Start the pprof server.
	go func() {
		_ = http.ListenAndServe(fmt.Sprintf("localhost:%d", *pprofPort), nil)
	}()

	if err := run(*binaryPath); err != nil {
		log.Errorf("Error: %v", err)
		log.Flush()
		os.Exit(1)
	}
}

func run(binaryPath string) error {
	log.Infof("Analyzing binary: %s", binaryPath)
	start := time.Now()
	symBuilder, err := symdb.NewSymDBBuilder(binaryPath)
	if err != nil {
		return err
	}
	opt := symdb.ExtractScopeAllSymbols
	if *onlyFirstParty {
		log.Infof("Extracting only 1st party symbols")
		opt = symdb.ExtractScopeModulesFromSameOrg
	}

	// Start tracing if we were asked to.
	tracing := *traceFile != ""
	if tracing {
		log.Infof("Tracing symbol extraction to %s", *traceFile)
		f, err := os.OpenFile(*traceFile, os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return fmt.Errorf("failed to open trace file %s: %w", *traceFile, err)
		}
		defer func() {
			_ = f.Close()
		}()
		if err := trace.Start(f); err != nil {
			return fmt.Errorf("failed to start trace: %w", err)
		}
		defer trace.Stop()
	}

	symbols, err := symBuilder.ExtractSymbols(opt)
	if err != nil {
		return err
	}
	trace.Stop()
	log.Infof("Symbol extraction completed in %s.", time.Since(start))
	stats := statsFromSymbols(symbols)
	log.Infof("Symbol statistics for %s: %+v", binaryPath, stats)
	if !*silent {
		symbols.Serialize(symdbutil.MakePanickingWriter(os.Stdout))
	} else {
		log.Infof("--silent specified; symbols not serialized.")
	}

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
	for _, pkg := range s.Packages {
		s := pkg.Stats()
		stats.numTypes += s.NumTypes
		stats.numFunctions += s.NumFunctions
		stats.numSourceFiles += s.NumSourceFiles
	}
	return stats
}
