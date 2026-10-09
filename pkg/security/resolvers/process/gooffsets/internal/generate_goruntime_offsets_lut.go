// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package main is responsible for extracting Go runtime struct offsets
// from DWARF data across multiple Go versions.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"go/format"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/network/go/dwarfutils"
	"github.com/DataDog/datadog-agent/pkg/network/go/goversion"
	"github.com/DataDog/datadog-agent/pkg/network/go/lutgen"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

var (
	outFlag            = flag.String("out", "", "output Go source file path")
	minGoVersionFlag   = flag.String("min-go", "", "min Go version")
	testProgramFlag    = flag.String("test-program", "", "path to test program to compile")
	archFlag           = flag.String("arch", "", "list of Go architectures")
	packageFlag        = flag.String("package", "", "package to use when generating source")
	sharedBuildDirFlag = flag.String("shared-build-dir", "", "shared directory to cache Go versions")
)

// swissMapsGoVersion is the first Go version using Swiss tables, which have no runtime.hmap.
var swissMapsGoVersion, _ = goversion.NewGoVersion("1.24")

// runtimeField is the DWARF lookup of one gooffsets.GoRuntimeOffsets field.
// The generator renders the struct by field name instead of importing gooffsets,
// so that it still builds when the file it generates is broken.
type runtimeField struct {
	name       string
	structName string
	fieldName  string
	// hmap fields only exist before Swiss tables and are left at 0 after.
	hmap bool
}

// runtimeFields lists the fields of gooffsets.GoRuntimeOffsets, in declaration order.
var runtimeFields = []runtimeField{
	{"MOffset", "runtime.g", "m", false},
	{"MGsignal", "runtime.m", "gsignal", false},
	{"Curg", "runtime.m", "curg", false},
	{"Labels", "runtime.g", "labels", false},
	{"HmapCount", "runtime.hmap", "count", true},
	{"HmapLog2BucketCount", "runtime.hmap", "B", true},
	{"HmapBuckets", "runtime.hmap", "buckets", true},
}

// newestGoVersion is the newest Go version inspected, from which MaxGoVersion is generated.
// lutgen inspects binaries concurrently.
var (
	newestGoVersionMu sync.Mutex
	newestGoVersion   goversion.GoVersion
)

// This program is intended to be called from go generate.
// It generates an implementation of:
// `func GetGoRuntimeOffsets(version goversion.GoVersion, goarch string) (GoRuntimeOffsets, error)`
// by compiling a test binary against multiple versions of Go and scanning the debug symbols
func main() {
	flag.Parse()

	outputFile, err := filepath.Abs(*outFlag)
	if err != nil {
		log.Fatalf("unable to get absolute path to %q: %s", *outFlag, err)
	}

	minGoVersion, err := goversion.NewGoVersion(*minGoVersionFlag)
	if err != nil {
		log.Fatalf("unable to parse min Go version %q", *minGoVersionFlag)
	}

	goArches := strings.Split(*archFlag, ",")

	ctx := context.Background()

	// Trap SIGINT to cancel the context
	ctx, cancel := context.WithCancel(ctx)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	defer func() {
		signal.Stop(c)
		cancel()
	}()
	go func() {
		select {
		case <-c:
			cancel()
		case <-ctx.Done():
		}
	}()

	err = run(ctx, outputFile, minGoVersion, goArches, *packageFlag, *testProgramFlag, *sharedBuildDirFlag)
	if err != nil {
		log.Fatalf("error generating lookup table: %s", err)
	}

	log.Printf("successfully generated lookup table at %s", outputFile)
}

func run(
	ctx context.Context,
	outputFile string,
	minGoVersion goversion.GoVersion,
	goArches []string,
	pkg string,
	testProgramPath string,
	sharedBuildDir string,
) error {
	if err := os.MkdirAll(filepath.Dir(outputFile), 0755); err != nil {
		return err
	}

	// Create a temp directory for the output files
	outDir, err := os.MkdirTemp("", "goruntime_offsets_lut_out_*")
	if err != nil {
		return fmt.Errorf("error creating temp out dir: %w", err)
	}
	defer os.RemoveAll(outDir)

	generator := &lutgen.LookupTableGenerator{
		Package:                pkg,
		MinGoVersion:           minGoVersion,
		Architectures:          goArches,
		CompilationParallelism: 1,
		LookupFunctions: []lutgen.LookupFunction{{
			Name:            "GetGoRuntimeOffsets",
			OutputType:      "GoRuntimeOffsets",
			OutputZeroValue: "GoRuntimeOffsets{}",
			DocComment:      `GetGoRuntimeOffsets gets the offsets of the Go runtime struct fields used by the ebpf-profiler`,
			RenderValue: func(v interface{}) string {
				offsets := v.([]uint32)
				values := make([]string, len(runtimeFields))
				for i, f := range runtimeFields {
					values[i] = fmt.Sprintf("%s: %d", f.name, offsets[i])
				}
				return "GoRuntimeOffsets{" + strings.Join(values, ", ") + "}"
			},
		}},
		InspectBinary:    inspectBinary,
		TestProgramPath:  testProgramPath,
		InstallDirectory: sharedBuildDir,
		OutDirectory:     outDir,
	}
	// Render to a buffer so that the output file is only written once complete.
	var buf bytes.Buffer
	if err := generator.Run(ctx, &buf); err != nil {
		return err
	}

	// MaxGoVersion is exclusive: the minor after the newest Go version the table was generated from.
	fmt.Fprintf(&buf, `
// MaxGoVersion is the first Go version newer than the ones the lookup table was generated from.
var MaxGoVersion = goversion.GoVersion{Major: %d, Minor: %d, Rev: 0}
`, newestGoVersion.Major, newestGoVersion.Minor+1)

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("error while formatting generated source: %w", err)
	}
	return os.WriteFile(outputFile, formatted, 0644)
}

func inspectBinary(binary lutgen.Binary) (interface{}, error) {
	file, err := os.Open(binary.Path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	elfFile, err := safeelf.NewFile(file)
	if err != nil {
		return nil, err
	}
	dwarfData, err := elfFile.DWARF()
	if err != nil {
		return nil, err
	}
	finder := dwarfutils.NewTypeFinder(dwarfData)

	swissMaps := binary.GoVersion.AfterOrEqual(swissMapsGoVersion)
	offsets := make([]uint32, len(runtimeFields))
	for i, f := range runtimeFields {
		if f.hmap && swissMaps {
			continue
		}
		offset, err := finder.FindStructFieldOffset(f.structName, f.fieldName)
		if err != nil {
			return nil, err
		}
		offsets[i] = uint32(offset)
	}

	newestGoVersionMu.Lock()
	if !newestGoVersion.AfterOrEqual(binary.GoVersion) {
		newestGoVersion = binary.GoVersion
	}
	newestGoVersionMu.Unlock()

	log.Printf("found runtime offsets for (go%s, %s)", binary.GoVersion, binary.Architecture)
	return offsets, nil
}
