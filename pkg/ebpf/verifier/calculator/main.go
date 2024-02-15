// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package main exercises BuildVerifierStats and outputs the result as a JSON to stdout
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/ebpf/verifier"
	"github.com/cilium/ebpf/rlimit"
)

type filters []string

func (f *filters) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func (i *filters) String() string {
	return fmt.Sprintf("%s", *i)
}

func main() {
	var err error
	var filterFiles filters
	var filterPrograms filters

	debug := flag.Bool("debug", false, "Calculate statistics of debug builds")
	flag.Var(&filterFiles, "filter-file", "Files to load ebpf programs from")
	flag.Var(&filterPrograms, "filter-prog", "Only return statistics for programs matching one of these regex pattern")
	flag.Parse()

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

	objectFiles := make(map[string]string)
	directory := os.Getenv("DD_SYSTEM_PROBE_BPF_DIR")
	if directory == "" {
		log.Fatalf("DD_SYSTEM_PROBE_BPF_DIR env var not set")
	}
	if err := filepath.WalkDir(directory, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		if len(filterFiles) > 0 {
			found := false
			for _, f := range filterFiles {
				if strings.TrimSuffix(d.Name(), ".o") == f {
					found = true
				}
			}
			if !found {
				return nil
			}
		}

		if skipDebugBuilds(path) || !strings.HasSuffix(path, ".o") {
			return nil
		}
		coreFile := filepath.Join(directory, "co-re", d.Name())
		if _, err := os.Stat(coreFile); err == nil {
			objectFiles[d.Name()] = coreFile
			return nil
		}

		// if not co-re file present then save normal path
		if _, ok := objectFiles[d.Name()]; !ok {
			objectFiles[d.Name()] = path
		}
		return nil
	}); err != nil {
		log.Fatalf("failed to walk directory %s: %v", directory, err)
	}

	var files []string
	// copy object files to temp directory with the correct permissions
	// loader code expects object files to be owned by root.
	for _, path := range objectFiles {
		src, err := os.Open(path)
		if err != nil {
			log.Fatalf("failed to open file %q for copying: %v", path, err)
		}
		defer src.Close()

		dstPath := filepath.Join(os.TempDir(), filepath.Base(path))
		if err := os.RemoveAll(dstPath); err != nil {
			log.Fatalf("failed to remove old file at %q: %v", dstPath, err)
		}
		dst, err := os.Create(dstPath)
		if err != nil {
			log.Fatalf("failed to open destination file %q for copying: %v", dstPath, err)
		}
		defer dst.Close()

		if _, err := io.Copy(dst, src); err != nil {
			log.Fatalf("failed to copy file %q to %q: %v", path, dstPath, err)
		}

		files = append(files, dstPath)
	}

	var filterRegexp []*regexp.Regexp
	for _, filter := range filterPrograms {
		filterRegexp = append(filterRegexp, regexp.MustCompile(filter))
	}

	stats, _, err := verifier.BuildVerifierStats(files, filterRegexp)
	if err != nil {
		log.Fatalf("failed to build verifier stats: %v", err)
	}

	j, err := json.Marshal(stats)
	if err != nil {
		log.Fatalf("failed to marshal json %v", err)
	}
	fmt.Println(string(j))
}
