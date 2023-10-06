// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build ignore

package main

import (
	"context"
	"debug/elf"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/go-delve/delve/pkg/goversion"

	"github.com/DataDog/datadog-agent/pkg/network/go/dwarfutils"
	"github.com/DataDog/datadog-agent/pkg/network/go/lutgen"
)

var (
	outFlag            = flag.String("out", "", "output Go source file path")
	minGoVersionFlag   = flag.String("min-go", "", "min Go version")
	testProgramFlag    = flag.String("test-program", "", "path to test program to compile")
	archFlag           = flag.String("arch", "", "list of Go architectures")
	packageFlag        = flag.String("package", "", "package to use when generating source")
	sharedBuildDirFlag = flag.String("shared-build-dir", "", "shared directory to cache Go versions")
)

// This program is intended to be called from go generate.
// It generates an implementation of:
// `func GetGoroutineIDOffset(version goversion.GoVersion, goarch string) (uint64, error)`
// by compiling a test binary against multiple versions of Go and scanning the debug symbols
func main() {
	flag.Parse()

	outputFile, err := filepath.Abs(*outFlag)
	if err != nil {
		log.Fatalf("unable to get absolute path to %q: %s", *outFlag, err)
	}

	minGoVersion, ok := goversion.Parse(fmt.Sprintf("go%s", *minGoVersionFlag))
	if !ok {
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

	f, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer f.Close()

	// Create a temp directory for the output files
	outDir, err := os.MkdirTemp("", "goid_lut_out_*")
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
			Name:            "GetGoroutineIDOffset",
			OutputType:      "uint64",
			OutputZeroValue: "0",
			DocComment:      `GetGoroutineIDOffset gets the offset of the "goid" field in the "runtime.g" struct`,
		}},
		InspectBinary:    inspectBinary,
		TestProgramPath:  testProgramPath,
		InstallDirectory: sharedBuildDir,
		OutDirectory:     outDir,
	}
	err = generator.Run(ctx, f)
	if err != nil {
		return err
	}

	return nil
}

func inspectBinary(binary lutgen.Binary) (interface{}, error) {
	// Inspect the binary to find the offset.
	file, err := os.Open(binary.Path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	elfFile, err := elf.NewFile(file)
	if err != nil {
		return 0, err
	}
	dwarfData, err := elfFile.DWARF()
	if err != nil {
		return 0, err
	}
	finder := dwarfutils.NewTypeFinder(dwarfData)
	offset, err := finder.FindStructFieldOffset("runtime.g", "goid")
	if err != nil {
		return 0, err
	}

	log.Printf("found struct offset for (go%s, %s)", binary.GoVersionString, binary.Architecture)
	return offset, nil
}
