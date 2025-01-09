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
	"slices"
	"strings"

	"github.com/cilium/ebpf/rlimit"

	"github.com/DataDog/datadog-agent/pkg/ebpf/verifier"
)

type filters []string

func (f *filters) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func (f *filters) String() string {
	return fmt.Sprintf("%s", *f)
}

func main() {
	var err error
	var filterFiles filters
	var filterPrograms filters

	debug := flag.Bool("debug", false, "Calculate statistics of debug builds")
	nostats := flag.Bool("no-stats", false, "Do not gather verifier stats")
	lineComplexity := flag.Bool("line-complexity", false, "Calculate line complexity, extracting data from the verifier logs")
	verifierLogsDir := flag.String("verifier-logs", "", "Directory containing verifier logs. If not set, no logs will be saved.")
	summaryOutput := flag.String("summary-output", "ebpf-calculator/summary.json", "File where JSON with the summary will be written")
	complexityDataDir := flag.String("complexity-data-dir", "ebpf-calculator/complexity-data", "Directory where the complexity data will be written")
	constantsList := flag.Bool("constants", false, "Get json list of all user defined constants")
	constantsFile := flag.String("constants-file", "", "File containing list of known constants")
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
	hasAbsPaths := false
	for _, f := range filterFiles {
		if filepath.IsAbs(f) {
			objectFiles[filepath.Base(f)] = f
			hasAbsPaths = true
		}
	}
	filterFiles = slices.DeleteFunc(filterFiles, func(s string) bool {
		return filepath.IsAbs(s)
	})

	if err := filepath.WalkDir(directory, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		if skipDebugBuilds(path) || !strings.HasSuffix(path, ".o") {
			return nil
		}

		if len(filterFiles) > 0 || hasAbsPaths {
			found := false
			for _, f := range filterFiles {
				if d.Name() == f {
					found = true
					break
				}
			}
			if !found {
				return nil
			}
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

	// verify that the constants the stats gatherer is aware of match
	// the constants in the object files
	if *constantsFile != "" {
		constants := make(map[string]map[string]uint64)

		for _, file := range files {
			module := strings.Split(filepath.Base(file), ".")[0]
			constants[module] = make(map[string]uint64)

			if err := verifier.BuildConstantsList(file, constants[module]); err != nil {
				log.Fatalf("failed to build constant list for file %s: %v", file, err)
			}
		}

		knownConstants := make(map[string]map[string]uint64)
		j, err := os.ReadFile(*constantsFile)
		if err != nil {
			log.Fatalf("failed to read constants file %s: %v", *constantsFile, err)
		}
		if err := json.Unmarshal(j, &knownConstants); err != nil {
			log.Fatalf("failed to unmarshal contents of json file %s: %v", *constantsFile, err)
		}

		var toremove []string
		for file, _ := range knownConstants {
			if _, ok := constants[file]; !ok {
				toremove = append(toremove, file)
			}
		}
		for _, file := range toremove {
			delete(knownConstants, file)
		}

		if len(constants) != len(knownConstants) {
			log.Fatalf("set of files present does not match the known set of files")
		}

		for f, consts := range constants {
			if _, ok := knownConstants[f]; !ok {
				log.Fatalf("no constants recorded for file %s.o", f)
			}

			if len(consts) != len(knownConstants[f]) {
				log.Fatalf("set of constants in file %s.o do no match known constants", f)
			}

			for c, _ := range consts {
				if _, ok := knownConstants[f][c]; !ok {
					log.Fatalf("constants %s for file %s.o not known", c, f)
				}
			}
		}
	}

	if *constantsList {
		ls := make(map[string]map[string]uint64)
		for _, file := range files {
			module := strings.Split(filepath.Base(file), ".")[0]
			ls[module] = make(map[string]uint64)

			if err := verifier.BuildConstantsList(file, ls[module]); err != nil {
				log.Fatalf("failed to build constant list for file %s: %v", file, err)
			}
		}

		jsonFile := filepath.Join("ebpf-calculator", "constants.json")
		j, err := json.MarshalIndent(ls, "", "	")
		if err != nil {
			log.Fatalf("failed to marshal constants into json list: %v", err)
		}

		log.Printf("Writing constants to %s", jsonFile)
		if err := os.MkdirAll(filepath.Dir(jsonFile), 0755); err != nil {
			log.Fatalf("failed to create directory %s: %v", filepath.Dir(jsonFile), err)
		}
		if err := os.WriteFile(jsonFile, j, 0644); err != nil {
			log.Fatalf("failed to write json file %s: %v", jsonFile, err)
		}
	}

	if *nostats {
		return
	}

	var filterRegexp []*regexp.Regexp
	for _, filter := range filterPrograms {
		filterRegexp = append(filterRegexp, regexp.MustCompile(filter))
	}

	statsOpts := verifier.StatsOptions{
		ObjectFiles:        files,
		VerifierLogsDir:    *verifierLogsDir,
		DetailedComplexity: *lineComplexity,
		FilterPrograms:     filterRegexp,
	}

	results, _, err := verifier.BuildVerifierStats(&statsOpts)
	if err != nil {
		log.Fatalf("failed to build verifier stats: %v", err)
	}

	j, err := json.Marshal(results.Stats)
	if err != nil {
		log.Fatalf("failed to marshal json %v", err)
	}

	if *summaryOutput != "" {
		log.Printf("Writing summary to %s", *summaryOutput)
		if err := os.MkdirAll(filepath.Dir(*summaryOutput), 0755); err != nil {
			log.Fatalf("failed to create directory %s: %v", filepath.Dir(*summaryOutput), err)
		}
		if err := os.WriteFile(*summaryOutput, j, 0666); err != nil {
			log.Fatalf("failed to write summary file %s: %v", *summaryOutput, err)
		}
	}

	if *lineComplexity {
		log.Printf("Writing complexity data to %s", *complexityDataDir)

		for objectFile, funcsPerSect := range results.FuncsPerSection {
			mappings, err := json.Marshal(funcsPerSect)
			if err != nil {
				log.Fatalf("failed to marshal funcs per section JSON: %v", err)
			}
			destPath := filepath.Join(*complexityDataDir, objectFile, "mappings.json")
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				log.Fatalf("failed to create directory %s: %v", filepath.Dir(destPath), err)
			}
			if err := os.WriteFile(destPath, mappings, 0644); err != nil {
				log.Fatalf("failed to write mappings data: %v", err)
			}
		}

		for progName, data := range results.Complexity {
			contents, err := json.Marshal(data)
			if err != nil {
				log.Fatalf("failed to marshal json for %s: %v", progName, err)
			}

			// The format of progName is "objectName/programName" so we need to make the
			// directory structure to ensure we can save the file in the correct place.
			destPath := filepath.Join(*complexityDataDir, fmt.Sprintf("%s.json", progName))
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				log.Fatalf("failed to create directory %s: %v", filepath.Dir(destPath), err)
			}
			if err := os.WriteFile(destPath, contents, 0644); err != nil {
				log.Fatalf("failed to write complexity data for %s: %v", progName, err)
			}
		}
	}
}
