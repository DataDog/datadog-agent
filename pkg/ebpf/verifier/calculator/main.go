// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main exercises BuildVerifierStats and outputs the result as a JSON to stdout
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/ebpf/verifier"
	"github.com/cilium/ebpf/rlimit"
)

var directory = flag.String("directory", "", "Directory containing ebpf object files")
var debug = flag.Bool("debug", false, "Calculate statistics of debug builds")

func main() {
	var objectFiles []string
	var err error

	flag.Parse()

	objDir := *directory
	skipDebugBuilds := func(path string) bool {
		debugBuild := strings.Contains(path, "-debug")
		if *debug {
			return !debugBuild
		}
		return debugBuild
	}

	if err := rlimit.RemoveMemlock(); err != nil {
		log.Fatalf("failed to remove memlock %v", err)
	}

	if err := filepath.WalkDir(objDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		if skipDebugBuilds(path) || !strings.HasSuffix(path, ".o") {
			return nil
		}
		objectFiles = append(objectFiles, path)

		return nil
	}); err != nil {
		log.Fatalf("failed to discover all object files: %v", err)
	}
	stats, err := verifier.BuildVerifierStats(objectFiles)
	if err != nil {
		log.Fatalf("failed to build verifier stats: %v", err)
	}

	j, err := json.Marshal(stats)
	if err != nil {
		log.Fatalf("failed to marshal json %v", err)
	}
	fmt.Println(string(j))
}
