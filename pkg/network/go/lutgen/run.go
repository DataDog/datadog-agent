// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package lutgen

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/network/go/goversion"
	"github.com/DataDog/datadog-agent/pkg/network/go/rungo"
	"github.com/DataDog/datadog-agent/pkg/network/go/rungo/matrix"
)

// LookupTableGenerator configures the process of generating
// a Go source file that implements lookup table(s) for any value
// based on an input Go version and Go architecture.
// This can be used to resolve values about a binary that change
// based on the Go version/architecture,
// by compiling and inspecting small test binaries ahead-of-type
// for each Go version/architecture and generating the lookup table(s)
// based on the results of this process.
//
// Each lookup table is implemented as a function,
// which are configured in LookupFunction types.
type LookupTableGenerator struct {
	Package                string
	MinGoVersion           goversion.GoVersion
	Architectures          []string
	CompilationParallelism int
	LookupFunctions        []LookupFunction
	ExtraImports           []string
	InspectBinary          func(binary Binary) (interface{}, error)
	TestProgramPath        string
	InstallDirectory       string
	OutDirectory           string

	allBinaries   []Binary
	allBinariesMu sync.Mutex
}

// Binary wraps the information about a single compiled test binary
// that is given to the inspection callback.
type Binary struct {
	Architecture string
	GoVersion    goversion.GoVersion
	Path         string
}

type architectureVersion struct {
	architecture string
	version      goversion.GoVersion
}

// Run runs the generator to completion,
// writing the generated Go source code to the given writer.
// If an error occurs installing Go toolchain versions,
// compiling the test program, or running the inspection function
// (or if the context is cancelled),
// then the function will return early.
func (g *LookupTableGenerator) Run(ctx context.Context, writer io.Writer) error {
	versions, err := g.getVersions(ctx)
	if err != nil {
		return err
	}

	// Create a folder to store the compiled binaries
	err = os.MkdirAll(g.OutDirectory, 0o777)
	if err != nil {
		return err
	}

	log.Println("running lookup table generation")
	log.Printf("architectures: %v", g.Architectures)
	sortedVersions := make([]goversion.GoVersion, len(versions))
	copy(sortedVersions, versions)
	sort.Slice(sortedVersions, func(x, y int) bool {
		return !sortedVersions[x].AfterOrEqual(sortedVersions[y])
	})
	log.Println("versions:")
	for _, v := range sortedVersions {
		log.Printf("- %s", v)
	}

	// Create a matrix runner to build the test program
	// against each combination of Go version and architecture.
	matrixRunner := &matrix.Runner{
		Parallelism:      g.CompilationParallelism,
		Versions:         versions,
		Architectures:    g.Architectures,
		InstallDirectory: g.InstallDirectory,
		GetCommand:       g.getCommand,
	}
	err = matrixRunner.Run(ctx)
	if err != nil {
		return err
	}

	// For all of the output binaries, run the inspection logic
	resultTable, err := g.inspectAllBinaries(ctx)
	if err != nil {
		return err
	}

	// For each lookup function, prepare the template arguments
	lookupFunctionArgs := []lookupFunctionTemplateArgs{}
	for _, lookupFn := range g.LookupFunctions {
		lookupFunctionArgs = append(lookupFunctionArgs, lookupFn.argsFromResultTable(resultTable))
	}

	// Construct the overall template args struct and render it
	args := templateArgs{
		Imports: append([]string{
			"fmt",
			"github.com/go-delve/delve/pkg/goversion",
		}, g.ExtraImports...),
		Package:                g.Package,
		LookupFunctions:        lookupFunctionArgs,
		MinGoVersion:           g.MinGoVersion,
		SupportedArchitectures: g.Architectures,
	}
	return args.Render(writer)
}

type majorMinorPair struct {
	major int
	minor int
}

func (g *LookupTableGenerator) getVersions(ctx context.Context) ([]goversion.GoVersion, error) {
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
	includedVersions := make(map[majorMinorPair]struct{})
	for _, version := range allVersions {
		if version.Rev != 0 ||
			version.Proposal != "" ||
			!version.AfterOrEqual(g.MinGoVersion) {
			continue
		}

		versions = append(versions, version)
		includedVersions[majorMinorPair{version.Major, version.Minor}] = struct{}{}
	}

	// Then, if there were any major,minor versions
	// that existed somewhere in the list of downloaded versions
	// but didn't have a 1.X.0 release, include them (including beta or RC versions).
	highestNonZeroRelease := make(map[majorMinorPair]goversion.GoVersion)
	for _, version := range allVersions {
		if _, ok := includedVersions[majorMinorPair{version.Major, version.Minor}]; !ok && version.AfterOrEqual(g.MinGoVersion) {
			// This version is a candidate to be its major,minor pair's highest beta/RC/rev!=0 version.
			if existing, ok := highestNonZeroRelease[majorMinorPair{version.Major, version.Minor}]; ok {
				if existing.AfterOrEqual(version) {
					// There is already a newer version
					continue
				}
			}
			highestNonZeroRelease[majorMinorPair{version.Major, version.Minor}] = version
		}
	}

	for _, v := range highestNonZeroRelease {
		versions = append(versions, v)
	}

	return versions, nil
}

func (g *LookupTableGenerator) addBinary(binary Binary) {
	g.allBinariesMu.Lock()
	defer g.allBinariesMu.Unlock()

	g.allBinaries = append(g.allBinaries, binary)
}

func (g *LookupTableGenerator) getAllBinaries() []Binary {
	g.allBinariesMu.Lock()
	defer g.allBinariesMu.Unlock()

	newSlice := make([]Binary, len(g.allBinaries))
	copy(newSlice, g.allBinaries)
	return newSlice
}

func (g *LookupTableGenerator) getCommand(ctx context.Context, version goversion.GoVersion, arch, goExe string) *exec.Cmd {
	outPath := filepath.Join(g.OutDirectory, fmt.Sprintf("%s.go%s", arch, version))

	// Store the binary struct in a list so that it can later be opened.
	// If the command ends up failing, this will be ignored
	// and the entire lookup table generation will exit early.
	g.addBinary(Binary{
		Path:         outPath,
		GoVersion:    version,
		Architecture: arch,
	})

	command := exec.CommandContext(
		ctx,
		"go",
		"build",
		"-o", outPath,
		"--",
		g.TestProgramPath,
	)

	// Set the GOPATH and GOCACHE variables.
	// Make sure to resolve the absolute path of install directory first.
	installDirectoryAbs, err := filepath.Abs(g.InstallDirectory)
	if err != nil {
		log.Printf("error install directory at %q: %v", g.InstallDirectory, err)
		return nil
	}
	command.Env = append(command.Env, fmt.Sprintf("%s=%s", "GOPATH", filepath.Join(installDirectoryAbs, "build-gopath")))
	command.Env = append(command.Env, fmt.Sprintf("%s=%s", "GOCACHE", filepath.Join(installDirectoryAbs, "build-gocache")))
	command.Env = append(command.Env, fmt.Sprintf("%s=%s", "GOARCH", arch))
	// The $HOME directory needs to be set to the Go installation directory
	command.Env = append(command.Env, fmt.Sprintf("%s=%s", "HOME", filepath.Join(g.InstallDirectory, "install")))
	command.Path = goExe

	// Add in the normal PATH environment variable
	// so that Go can resolve gcc in case it needs to use cgo.
	command.Env = append(command.Env, fmt.Sprintf("%s=%s", "PATH", os.Getenv("PATH")))

	err = setupGoModule(ctx, command, g.TestProgramPath)
	if err != nil {
		log.Printf("error setting up go module for  %s (%s): %s", g.TestProgramPath, version, err)
		return nil
	}

	return command
}

// inspectAllBinaries runs the inspection function for each binary in parallel,
// returning a "result table" that maps architecture,version pairs
// to the result of the inspection.
func (g *LookupTableGenerator) inspectAllBinaries(ctx context.Context) (map[architectureVersion]interface{}, error) {
	// Get all of the binaries that were generated from the matrix runner
	binaries := g.getAllBinaries()

	results := make(chan struct {
		bin    Binary
		result interface{}
		err    error
	})
	for _, bin := range binaries {
		go func(bin Binary) {
			result, err := g.InspectBinary(bin)
			results <- struct {
				bin    Binary
				result interface{}
				err    error
			}{bin, result, err}
		}(bin)
	}

	resultTable := make(map[architectureVersion]interface{})
	for range binaries {
		select {
		case result := <-results:
			if result.err != nil {
				// Bail early and return
				return nil, fmt.Errorf("error inspecting binary for (Go version, arch pair) (go%s, %s) at %q: %w", result.bin.GoVersion, result.bin.Architecture, result.bin.Path, result.err)
			}

			resultTable[architectureVersion{result.bin.Architecture, result.bin.GoVersion}] = result.result
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return resultTable, nil
}

func setupGoModule(ctx context.Context, cmd *exec.Cmd, programPath string) error {
	moduleDir, err := os.MkdirTemp("", "lut")
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

	// create go.mod file
	err = os.WriteFile(filepath.Join(moduleDir, "go.mod"), []byte("module foobar"), os.ModePerm)
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
