// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// configsender is an experimental standalone tool for the DSCVR-438 PoC.
// It reads a config file from disk, redacts known sensitive keys, builds the
// EvP envelope expected by the demoalpha-worker (DataDog/experimental#9989),
// and POSTs it to a configurable intake URL.
//
// This is NOT part of the agent binary. It is a thin Go port of the bash
// sender in DataDog/experimental, intended to validate the agent-side leg
// of the ingestion pipeline before any Phase-D architecture decision (see
// the DSCVR roadmap).
//
// Usage:
//
//	go run ./pkg/discovery/tools/configsender \
//	  --intake-url=https://... \
//	  --host-id=$(hostname) \
//	  --integration=redis \
//	  --source=app_native \
//	  /etc/redis/redis.conf
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

const maxFileSize = 256 * 1024

func main() {
	var (
		intakeURL   = flag.String("intake-url", "", "EvP intake URL (required, e.g. https://.../v2/track/demoalpha/org/47653)")
		apiKey      = flag.String("api-key", os.Getenv("DD_API_KEY"), "Datadog API key (or DD_API_KEY env)")
		hostID      = flag.String("host-id", "", "Host identifier (required)")
		integration = flag.String("integration", "", "Integration name (required, e.g. redis)")
		source      = flag.String("source", "app_native", "Config source: app_native | agent_integration")
		contentType = flag.String("content-type", "", "Override detected content_type (yaml | json | redis_conf)")
		noRedact    = flag.Bool("no-redact", false, "Skip sensitive-key redaction (NOT recommended)")
		dryRun      = flag.Bool("dry-run", false, "Print the envelope to stdout instead of POSTing")
	)
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s [flags] <config-file>...\n\nflags:\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}
	flag.Parse()

	paths := flag.Args()
	if len(paths) == 0 {
		flag.Usage()
		os.Exit(2)
	}
	if *hostID == "" || *integration == "" {
		log.Fatal("--host-id and --integration are required")
	}
	if !*dryRun && *intakeURL == "" {
		log.Fatal("--intake-url is required (or use --dry-run)")
	}
	switch *source {
	case "app_native", "agent_integration":
	default:
		log.Fatalf("--source must be app_native or agent_integration (got %q)", *source)
	}

	ctx := context.Background()
	exit := 0
	for _, path := range paths {
		if err := process(ctx, processArgs{
			path:        path,
			hostID:      *hostID,
			integration: *integration,
			source:      *source,
			contentType: *contentType,
			intakeURL:   *intakeURL,
			apiKey:      *apiKey,
			redact:      !*noRedact,
			dryRun:      *dryRun,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "ship %s: %v\n", path, err)
			exit = 1
		}
	}
	os.Exit(exit)
}

type processArgs struct {
	path        string
	hostID      string
	integration string
	source      string
	contentType string
	intakeURL   string
	apiKey      string
	redact      bool
	dryRun      bool
}

func process(ctx context.Context, a processArgs) error {
	ct := a.contentType
	if ct == "" {
		ct = detectContentType(a.integration, a.path)
	}
	if ct == "" {
		return fmt.Errorf("could not infer content_type for integration=%s path=%s; pass --content-type", a.integration, a.path)
	}

	raw, err := readCapped(a.path, maxFileSize)
	if err != nil {
		return err
	}
	if a.redact {
		raw = redactSensitive(raw)
	}

	body, hash, err := buildEnvelope(a.hostID, a.integration, a.source, a.path, ct, raw)
	if err != nil {
		return err
	}

	if a.dryRun {
		_, _ = os.Stdout.Write(body)
		_, _ = fmt.Fprintf(os.Stderr, "\n[dry-run] hash=%s integration=%s source=%s host=%s\n", hash[:12], a.integration, a.source, a.hostID)
		return nil
	}

	if err := postEnvelope(ctx, a.intakeURL, a.apiKey, body); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "sent: host=%s integration=%s source=%s file=%s hash=%s\n",
		a.hostID, a.integration, a.source, a.path, hash[:12])
	return nil
}

func readCapped(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, max))
}
