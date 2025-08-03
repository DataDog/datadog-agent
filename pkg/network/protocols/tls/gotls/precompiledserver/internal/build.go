// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ignore

package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/network/go/goversion"
	"github.com/DataDog/datadog-agent/pkg/network/go/rungo"
	"github.com/DataDog/datadog-agent/pkg/network/go/rungo/matrix"
)

var (
	outFlag            = flag.String("out", "", "output directory for compiled binaries")
	minGoVersionFlag   = flag.String("min-go", "1.13", "min Go version")
	testProgramFlag    = flag.String("test-program", "test_server.go", "path to test program to compile")
	archFlag           = flag.String("arch", "amd64,arm64", "comma-separated list of Go architectures")
	sharedBuildDirFlag = flag.String("shared-build-dir", "", "shared directory to cache Go versions")
)

func main() {
	flag.Parse()

	outputDir, err := filepath.Abs(*outFlag)
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

	err = run(ctx, outputDir, minGoVersion, goArches, *testProgramFlag, *sharedBuildDirFlag)
	if err != nil {
		log.Fatalf("error generating binaries: %s", err)
	}

	fmt.Printf("successfully generated binaries at %s\n", outputDir)
}

func run(
	ctx context.Context,
	outputDir string,
	minGoVersion goversion.GoVersion,
	goArches []string,
	testProgramPath string,
	sharedBuildDir string,
) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	versions, err := getVersions(ctx, minGoVersion)
	if err != nil {
		return fmt.Errorf("error getting Go versions: %w", err)
	}
	matrixRunner := &matrix.Runner{
		Parallelism:      1,
		Versions:         versions,
		Architectures:    goArches,
		InstallDirectory: sharedBuildDir,
		GetCommand: func(ctx context.Context, version goversion.GoVersion, arch string, goExe string) *exec.Cmd {
			outputName := fmt.Sprintf("https-go%s-%s", version, arch)
			outputPath := filepath.Join(outputDir, outputName)

			cmd := exec.CommandContext(ctx, goExe, "build",
				"-a", "-ldflags", "-extldflags=-static -w",
				"-tags", "netgo",
				"-o", outputPath,
				testProgramPath)

			cmd.Env = append(cmd.Env,
				"CGO_ENABLED=0",
				"GOOS=linux",
				"GOARCH="+arch,
				"GOPATH="+filepath.Join(sharedBuildDir, "build-gopath"),
				"GOCACHE="+filepath.Join(sharedBuildDir, "build-gocache"),
				"HOME="+filepath.Join(sharedBuildDir, "install"),
			)

			if err := setupGoModule(ctx, cmd, testProgramPath, version); err != nil {
				log.Printf("error setting up go module for  %s (%s): %s", testProgramPath, version, err)
				return nil
			}
			return cmd
		},
	}
	return matrixRunner.Run(ctx)
}

func getVersions(ctx context.Context, minVersion goversion.GoVersion) ([]goversion.GoVersion, error) {
	// Download a list of all of the go versions
	allRawVersions, err := rungo.ListGoVersions(ctx)
	if err != nil {
		return nil, err
	}

	// Parse each Go version to the struct form
	allVersions := []goversion.GoVersion{}
	for _, rawVersion := range allRawVersions {
		if version, err := goversion.NewGoVersion(rawVersion); err == nil {
			allVersions = append(allVersions, version)
		}
	}

	// Filter versions to those greater than the minimum,
	// and non-beta, RC, revision, or proposal versions.
	versions := []goversion.GoVersion{}
	for _, version := range allVersions {
		if version.Rev != 0 ||
			version.Proposal != "" ||
			!version.AfterOrEqual(minVersion) {
			continue
		}

		versions = append(versions, version)
	}

	return versions, nil
}

func setupGoModule(ctx context.Context, cmd *exec.Cmd, programPath string, version goversion.GoVersion) error {
	moduleDir, err := os.MkdirTemp("", "test-server")
	if err != nil {
		return err
	}

	// symlink test program
	err = os.MkdirAll(filepath.Join(moduleDir, filepath.Dir(programPath)), os.ModePerm)
	if err != nil {
		return err
	}
	absProgramPath, err := filepath.Abs(programPath)
	if err != nil {
		return err
	}
	err = os.Symlink(absProgramPath, filepath.Join(moduleDir, programPath))
	if err != nil {
		return err
	}

	// create go.mod file with appropriate Go version directive and dependency constraints
	// Use the format "1.X" for compatibility with older Go versions
	goVersionStr := fmt.Sprintf("1.%d", version.Minor)
	goModContent := fmt.Sprintf("module foobar\n\ngo %s\n", goVersionStr)

	err = os.WriteFile(filepath.Join(moduleDir, "go.mod"), []byte(goModContent), os.ModePerm)
	if err != nil {
		return fmt.Errorf("error creating go.mod file: %w", err)
	}

	// modify original `exec.Cmd` object by setting the `Dir` field to the one we created
	cmd.Dir = moduleDir

	// now run `go mod tidy`
	modCmd := exec.CommandContext(ctx, "go", "mod", "tidy")
	modCmd.Env = cmd.Env
	modCmd.Dir = cmd.Dir
	modCmd.Path = cmd.Path
	output, err := modCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error executing 'go mod tidy': %s\n%s", err, output)
	}

	return nil
}
