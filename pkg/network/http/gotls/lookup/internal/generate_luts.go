// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build ignore
// +build ignore

package main

import (
	"context"
	"debug/elf"
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/go-delve/delve/pkg/goversion"

	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
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
// It generates the following lookup tables:
// - `func GetWriteParams(version goversion.GoVersion, goarch string) ([]bininspect.ParameterMetadata, error)`
// - `func GetReadParams(version goversion.GoVersion, goarch string) ([]bininspect.ParameterMetadata, error)`
// - `func GetCloseParams(version goversion.GoVersion, goarch string) ([]bininspect.ParameterMetadata, error)`
// - `func GetTLSConnInnerConnOffset(version goversion.GoVersion, goarch string) (uint64, error)`
// - `func GetTCPConnInnerConnOffset(version goversion.GoVersion, goarch string) (uint64, error)`
// - `func GetConnFDOffset(version goversion.GoVersion, goarch string) (uint64, error)`
// - `func GetNetFD_PFDOffset(version goversion.GoVersion, goarch string) (uint64, error)`
// - `func GetFD_SysfdOffset(version goversion.GoVersion, goarch string) (uint64, error)`
// by compiling a test binary against multiple versions of Go and scanning the debug symbols.
// This assumes that these properties are constant given a Go version/architecture.
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

	fmt.Printf("successfully generated lookup table at %s\n", outputFile)
}

type inspectionResult struct {
	writePrams             []bininspect.ParameterMetadata
	readParams             []bininspect.ParameterMetadata
	closeParams            []bininspect.ParameterMetadata
	tlsConnInnerConnOffset uint64
	tcpConnInnerConnOffset uint64
	connFDOffset           uint64
	netFD_PFDOffset        uint64
	fd_SysfdOffset         uint64
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
		LookupFunctions: []lutgen.LookupFunction{
			{
				Name:            "GetWriteParams",
				OutputType:      "[]bininspect.ParameterMetadata",
				OutputZeroValue: "nil",
				DocComment:      `GetWriteParams gets the parameter metadata (positions/types) for crypto/tls.(*Conn).Write`,
				ExtractValue:    func(r interface{}) interface{} { return (r).(inspectionResult).writePrams },
			},
			{
				Name:            "GetReadParams",
				OutputType:      "[]bininspect.ParameterMetadata",
				OutputZeroValue: "nil",
				DocComment:      `GetReadParams gets the parameter metadata (positions/types) for crypto/tls.(*Conn).Read`,
				ExtractValue:    func(r interface{}) interface{} { return (r).(inspectionResult).readParams },
			},
			{
				Name:            "GetCloseParams",
				OutputType:      "[]bininspect.ParameterMetadata",
				OutputZeroValue: "nil",
				DocComment:      `GetWriteParams gets the parameter metadata (positions/types) for crypto/tls.(*Conn).Close`,
				ExtractValue:    func(r interface{}) interface{} { return (r).(inspectionResult).closeParams },
			},
			{
				Name:            "GetTLSConnInnerConnOffset",
				OutputType:      "uint64",
				OutputZeroValue: "0",
				DocComment:      `GetTLSConnInnerConnOffset gets the offset of the "conn" field in the "crypto/tls.Conn" struct`,
				ExtractValue:    func(r interface{}) interface{} { return (r).(inspectionResult).tlsConnInnerConnOffset },
			},
			{
				Name:            "GetTCPConnInnerConnOffset",
				OutputType:      "uint64",
				OutputZeroValue: "0",
				DocComment:      `GetTCPConnInnerConnOffset gets the offset of the "conn" field in the "net.TCPConn" struct`,
				ExtractValue:    func(r interface{}) interface{} { return (r).(inspectionResult).tcpConnInnerConnOffset },
			},
			{
				Name:            "GetConnFDOffset",
				OutputType:      "uint64",
				OutputZeroValue: "0",
				DocComment:      `GetConnFDOffset gets the offset of the "fd" field in the "net.conn" struct`,
				ExtractValue:    func(r interface{}) interface{} { return (r).(inspectionResult).connFDOffset },
			},
			{
				Name:            "GetNetFD_PFDOffset",
				OutputType:      "uint64",
				OutputZeroValue: "0",
				DocComment:      `GetNetFD_PFDOffset gets the offset of the "pfd" field in the "net.netFD" struct`,
				ExtractValue:    func(r interface{}) interface{} { return (r).(inspectionResult).netFD_PFDOffset },
			},
			{
				Name:            "GetFD_SysfdOffset",
				OutputType:      "uint64",
				OutputZeroValue: "0",
				DocComment:      `GetFD_SysfdOffset gets the offset of the "Sysfd" field in the "internal/poll.FD" struct`,
				ExtractValue:    func(r interface{}) interface{} { return (r).(inspectionResult).fd_SysfdOffset },
			},
		},
		ExtraImports: []string{
			// Need to import bininspect for the `[]bininspect.ParameterMetadata` type,
			// which is used as the output type on many of the lookup table functions.
			"github.com/DataDog/datadog-agent/pkg/network/go/bininspect",
		},
		InspectBinary:    inspectBinary,
		TestProgramPath:  testProgramPath,
		InstallDirectory: sharedBuildDir,
		OutDirectory:     outDir,
	}
	err = generator.Run(ctx, f)
	if err != nil {
		return err
	}

	// Run `gofmt -w -s` to simplify the generated file.
	// This simplifies expressions like `[]T{T{...}, T{...}}`
	// to `[]T{{...}, {...}}`.
	command := exec.CommandContext(ctx, "gofmt", "-l", "-w", "-s", "--", outputFile)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	log.Printf("%s %s", command.Path, strings.Join(command.Args[1:], " "))
	err = command.Run()
	if err != nil {
		return err
	}

	return nil
}

func inspectBinary(binary lutgen.Binary) (interface{}, error) {
	file, err := os.Open(binary.Path)
	if err != nil {
		return inspectionResult{}, err
	}
	defer file.Close()

	elfFile, err := elf.NewFile(file)
	if err != nil {
		return inspectionResult{}, err
	}

	// Inspect the binary using `binspect`
	config := bininspect.Config{
		Functions: []bininspect.FunctionConfig{
			{
				Name:                   "crypto/tls.(*Conn).Write",
				IncludeReturnLocations: false,
			},
			{
				Name:                   "crypto/tls.(*Conn).Read",
				IncludeReturnLocations: false,
			},
			{
				Name:                   "crypto/tls.(*Conn).Close",
				IncludeReturnLocations: false,
			},
		},
		StructOffsets: []bininspect.StructOffsetConfig{
			{
				StructName: "crypto/tls.Conn",
				FieldName:  "conn",
			},
			{
				StructName: "net.TCPConn",
				FieldName:  "conn",
			},
			{
				StructName: "net.conn",
				FieldName:  "fd",
			},
			{
				StructName: "net.netFD",
				FieldName:  "pfd",
			},
			{
				StructName: "internal/poll.FD",
				FieldName:  "Sysfd",
			},
		},
	}
	rawResult, err := bininspect.Inspect(elfFile, config)
	if err != nil {
		return inspectionResult{}, err
	}

	result := inspectionResult{}
	for _, s := range rawResult.StructOffsets {
		if s.StructName == "crypto/tls.Conn" && s.FieldName == "conn" {
			result.tlsConnInnerConnOffset = s.Offset
		} else if s.StructName == "net.TCPConn" && s.FieldName == "conn" {
			result.tcpConnInnerConnOffset = s.Offset
		} else if s.StructName == "net.conn" && s.FieldName == "fd" {
			result.connFDOffset = s.Offset
		} else if s.StructName == "net.netFD" && s.FieldName == "pfd" {
			result.netFD_PFDOffset = s.Offset
		} else if s.StructName == "internal/poll.FD" && s.FieldName == "Sysfd" {
			result.fd_SysfdOffset = s.Offset
		}
	}

	for _, s := range rawResult.Functions {
		if s.Name == "crypto/tls.(*Conn).Write" {
			result.writePrams = s.Parameters
		} else if s.Name == "crypto/tls.(*Conn).Read" {
			result.readParams = s.Parameters
		} else if s.Name == "crypto/tls.(*Conn).Close" {
			result.closeParams = s.Parameters
		}
	}

	log.Printf("extracted binary data for (go%s, %s)", binary.GoVersionString, binary.Architecture)
	return result, nil
}
