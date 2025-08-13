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
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"runtime/trace"
	"time"

	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb/symdbutil"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uploader"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	binaryPath     = flag.String("binary-path", "", "Path to the binary to analyze.")
	silent         = flag.Bool("silent", false, "If set, the collected symbols are not printed.")
	stream         = flag.Bool("stream", false, "Use the package streaming mode for parsing the debug info. This implies ignoring the inlined functions.")
	onlyFirstParty = flag.Bool("only-1stparty", false,
		"Only output symbols for \"1st party\" code (i.e. code from modules belonging "+
			"to the same GitHub org as the main one).")

	upload     = flag.Bool("upload", false, "If specified, the SymDB data will be uploaded through a trace-agent.")
	uploadSite = flag.String("upload-site", "", "The site to which SymDB data will be uploaded. "+
		"If neither -upload-site or -upload-url are specified, datad0g.com is used as the site.")
	uploadURL = flag.String("upload-url",
		"https://debugger-intake.datad0g.com/api/v2/debugger",
		"If specified, the SymDB data will be uploaded to this URL. "+
			"Either -upload-site or -upload-url must be set when -upload is specified.")
	uploadService = flag.String("service", "", "The service name to use when uploading SymDB data.")
	uploadEnv     = flag.String("env", "", "The environment name to use when uploading SymDB data.")
	uploadVersion = flag.String("version", "", "The version to use when uploading SymDB data.")
	uploadAPIKey  = flag.String("api-key", "", "The API key used to authenticate uploads.")

	pprofPort = flag.Int("pprof-port", 8081, "Port for pprof server.")
	traceFile = flag.String("trace", "", "Path to the file to save an execution trace to.")
)

func main() {
	flag.Parse()
	if *binaryPath == "" {
		fmt.Print(`Usage: symdbcli --binary-path <path-to-binary> [--only-1stparty] [--silent]
or
symdbcli --binary-path <path-to-binary> [--only-1stparty] --upload --service <service> --env <env> --version <version> --api-key <api-key> [--upload-site <site>]

The symbols from the specified binary will be extracted and either printed to stdout
(unless --silent is specified) or uploaded to the backend.
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

func run(binaryPath string) (retErr error) {
	log.Infof("Analyzing binary: %s", binaryPath)
	start := time.Now()
	scope := symdb.ExtractScopeAllSymbols

	var uploadURLParsed *url.URL
	if *upload {
		// Upload implies silent mode.
		*silent = true

		if *uploadURL != "" && *uploadSite != "" {
			return fmt.Errorf("only one of -upload-url or -upload-side must be specified")
		}
		if *uploadSite == "" {
			*uploadSite = "datad0g.com"
		}
		if *uploadURL == "" {
			*uploadURL = fmt.Sprintf("https://debugger-intake.%s/api/v2/debugger", *uploadSite)
		}

		if *uploadAPIKey == "" {
			return fmt.Errorf("-api-key must be specified when -upload is used")
		}
		var err error

		uploadURLParsed, err = url.Parse(*uploadURL)
		if err != nil {
			return fmt.Errorf("failed to parse upload URL %s: %w", *uploadURL, err)
		}

		if *uploadService == "" || *uploadEnv == "" || *uploadVersion == "" {
			return fmt.Errorf("when --upload is specified, --service, --env and --version must also be specified")
		}
	}

	if *onlyFirstParty {
		log.Infof("Extracting only 1st party symbols")
		scope = symdb.ExtractScopeModulesFromSameOrg
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

	var symbols symdb.Symbols
	opt := symdb.ExtractOptions{
		Scope:                   scope,
		IncludeInlinedFunctions: !*stream,
	}

	var up *uploader.SymDBUploader
	if *upload {
		up = uploader.NewSymDBUploader(
			*uploadService, *uploadEnv, *uploadVersion,
			fmt.Sprintf("manual-upload-%d", rand.Intn(1000)),
			uploader.WithURL(uploadURLParsed),
			uploader.WithHeader("DD-EVP-ORIGIN", "symdb-cli"),
			uploader.WithHeader("DD-EVP-ORIGIN-VERSION", "0.1"),
			uploader.WithHeader("DD-API-KEY", *uploadAPIKey),
		)
		defer func() {
			log.Infof("Waiting for upload to complete.")
			if err := up.Stop(); err != nil {
				if retErr == nil {
					retErr = err
				}
			}
		}()
	}

	if !*stream {
		var err error
		symbols, err = symdb.ExtractSymbols(binaryPath, opt)
		if err != nil {
			return err
		}
		if up != nil {
			for _, pkg := range symbols.Packages {
				if err := up.Enqueue(pkg); err != nil {
					log.Errorf("Failed to enqueue package %s for upload: %v", pkg.Name, err)
				}
			}
		}
	} else {
		it, err := symdb.PackagesIterator(binaryPath, opt)
		if err != nil {
			return err
		}
		for pkg, err := range it {
			if err != nil {
				return err
			}

			if up != nil {
				if err := up.Enqueue(pkg); err != nil {
					log.Errorf("Failed to enqueue package %s for upload: %v", pkg.Name, err)
				}
			}

			symbols.Packages = append(symbols.Packages, pkg)
		}
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
	sourceFiles := make(map[string]struct{})
	for _, pkg := range s.Packages {
		s := pkg.Stats(sourceFiles)
		stats.numTypes += s.NumTypes
		stats.numFunctions += s.NumFunctions
	}
	stats.numSourceFiles = len(sourceFiles)
	return stats
}
