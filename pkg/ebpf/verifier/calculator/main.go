// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main exercises BuildVerifierStats and outputs the result as a JSON to stdout
package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/ebpf/verifier"
	"github.com/cilium/ebpf/rlimit"
)

func main() {
	var objectFiles []string
	var err error

	if len(os.Args[1:]) < 1 {
		panic("please use './main <object-files-dir>'")
	}
	directory := os.Args[1]

	if err := rlimit.RemoveMemlock(); err != nil {
		log.Fatalf("failed to remove memlock %v", err)
	}

	if err := filepath.WalkDir(directory, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		if strings.Contains(path, "-debug") || !strings.HasSuffix(path, ".o") {
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
