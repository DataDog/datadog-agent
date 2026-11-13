// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Command update-ddot-manifest rewrites the pinned component versions in a DDOT
// OCB manifest to match a target Datadog Agent release, using a static
// compatibility table. Custom components (modules outside the OpenTelemetry
// core/contrib and Datadog module prefixes) are detected and reported but left
// untouched, since their compatibility with a new release can only be verified
// by their maintainer.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

// defaultManifest is the DDOT OCB manifest location, relative to the repo root.
const defaultManifest = "comp/otelcol/collector-contrib/impl/manifest.yaml"

func main() {
	agentVersion := flag.String("agent-version", "", "target Datadog Agent version, e.g. 7.83.2 (required)")
	manifestPath := flag.String("manifest", defaultManifest, "path to the OCB manifest to update")
	outputPath := flag.String("output", "", "path to write the updated manifest (default: overwrite --manifest)")
	dryRun := flag.Bool("dry-run", false, "report changes and custom components without writing any file")
	flag.Parse()

	if *agentVersion == "" {
		fmt.Fprintln(os.Stderr, "usage: update-ddot-manifest --agent-version <version> [--manifest <path>] [--output <path>] [--dry-run]")
		fmt.Fprintln(os.Stderr, "\nflags:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	out := *outputPath
	if out == "" {
		out = *manifestPath
	}
	if err := run(*agentVersion, *manifestPath, out, *dryRun); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// run performs the update end to end: look up the target versions, read and
// rewrite the manifest, report the result, and (unless dry-run) write it out.
func run(agentVersion, manifestPath, outputPath string, dryRun bool) error {
	v, err := lookupVersion(agentVersion)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("reading manifest: %w", err)
	}

	updated, res, err := updateManifest(data, v)
	if err != nil {
		return err
	}

	reportChanges(os.Stdout, agentVersion, v, res)
	warnCustom(os.Stderr, agentVersion, res.custom)

	if dryRun {
		fmt.Fprintln(os.Stdout, "\n(dry-run) no files written")
		return nil
	}
	if err := os.WriteFile(outputPath, updated, 0o644); err != nil {
		return fmt.Errorf("writing manifest: %w", err)
	}
	fmt.Fprintf(os.Stdout, "\nWrote updated manifest to %s\n", outputPath)
	return nil
}

// reportChanges prints the target versions and every version rewrite applied.
func reportChanges(w io.Writer, agentVersion string, v ddotVersion, res *updateResult) {
	fmt.Fprintf(w, "DDOT manifest update for Agent %s (OCB %s, core-stable %s, contrib %s)\n",
		agentVersion, v.OCBVersion, v.CoreStableVersion, v.ContribVersion)
	if len(res.changed) == 0 {
		fmt.Fprintln(w, "  no version changes")
		return
	}
	fmt.Fprintf(w, "  %d version change(s):\n", len(res.changed))
	for _, c := range res.changed {
		fmt.Fprintf(w, "    %s: %s -> %s\n", c.module, c.oldV, c.newV)
	}
}

// warnCustom prints a compatibility warning listing the custom components that
// were left untouched. It is a no-op when none were detected.
func warnCustom(w io.Writer, agentVersion string, custom []string) {
	if len(custom) == 0 {
		return
	}
	fmt.Fprintf(w, "\nWARNING: %d custom component(s) detected and left UNCHANGED.\n", len(custom))
	fmt.Fprintln(w, "These components are not maintained by Datadog. You must verify their")
	fmt.Fprintf(w, "compatibility with Agent %s and update their versions yourself:\n", agentVersion)
	for _, m := range custom {
		fmt.Fprintf(w, "  - %s\n", m)
	}
}
