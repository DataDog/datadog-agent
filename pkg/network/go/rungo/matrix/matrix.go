// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package matrix provides a utility for running a single Go command
package matrix

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/network/go/goversion"
	"github.com/DataDog/datadog-agent/pkg/network/go/rungo"
)

// Runner contains the logic used to run a single Go command
// on a matrix of Go toolchain version and Go architecture combinations.
// It handles downloading the Go toolchains for each version
// to the provided `InstallDirectory`, which can be cached to speed up future runs.
type Runner struct {
	// The number of single architecture-version runners that are active at any time.
	Parallelism int
	// List of Go versions to run commands on.
	// These should all be non-beta/RC versions.
	Versions []goversion.GoVersion
	// List of Go architectures (values for GOARCH) to run commands on.
	Architectures []string
	// The root directory that the matrix runner should use to install the Go toolchain versions.
	InstallDirectory string
	// Constructs the command to run for the single architecture-version runner.
	// The implementation should use `exec.CommandContext` and pass in the supplied context
	// to ensure that the command is cancelled if the runner exits early.
	// The GOARCH environment variable is automatically injected with the appropriate value.
	// Additionally, the value of `Path` is ignored
	// and replaced with the path to the installed Go binary.
	// Finally, due to a quirk with how the toolchain install path is resolved,
	// the HOME environment variable is replaced with a synthetic value.
	//
	// To skip a version-architecture pair, return `nil` for this function.
	GetCommand func(ctx context.Context, version goversion.GoVersion, arch string, goExe string) *exec.Cmd
}

type architectureVersion struct {
	architecture string
	version      goversion.GoVersion
}

// Run runs the matrix runner to completion,
// exiting early (and dumping the output) if any of the individual commands fail.
// Otherwise, it runs a command for every combination of Go version and architecture.
func (r *Runner) Run(ctx context.Context) error {
	if r.Parallelism <= 0 {
		return fmt.Errorf("cannot run with negative/zero Parallelism (%d)", r.Parallelism)
	}
	if r.GetCommand == nil {
		return fmt.Errorf("GetCommand is required")
	}
	if r.InstallDirectory == "" {
		return fmt.Errorf("InstallDirectory is required")
	}

	// If the install directory is not absolute, resolve it
	if !filepath.IsAbs(r.InstallDirectory) {
		abs, err := filepath.Abs(r.InstallDirectory)
		if err != nil {
			return fmt.Errorf("could not resolve absolute path of install directory %q: %w", r.InstallDirectory, err)
		}
		r.InstallDirectory = abs
	}

	// First, install all Go toolchain versions
	// to produce a map of go version -> "go" binary path
	log.Printf("installing Go toolchain versions in %s", r.getInstallLocation())
	executables, err := r.installAllVersions(ctx)
	if err != nil {
		return fmt.Errorf("error while installing Go toolchains for matrix runner: %w", err)
	}

	combinations := getCombinations(r.Versions, r.Architectures)
	if len(combinations) == 0 {
		// Nothing to run
		return nil
	}

	jobs := make(chan architectureVersion, len(combinations))
	results := make(chan struct {
		version architectureVersion
		err     error
	})

	cancellableContext, cancel := context.WithCancel(ctx)
	defer cancel()

	log.Printf("running commands on matrix of toolchains with parallelism of %d", r.Parallelism)

	for i := 0; i < r.Parallelism; i++ {
		go func() {
			for j := range jobs {
				err := r.runSingleCommand(cancellableContext, executables[j.version], j.version, j.architecture)
				results <- struct {
					version architectureVersion
					err     error
				}{j, err}
			}
		}()
	}

	for _, job := range combinations {
		jobs <- job
	}
	close(jobs)

	for range combinations {
		select {
		case result := <-results:
			if result.err != nil {
				// Bail early and return
				cancel()
				return result.err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	log.Println("done running commands on matrix of toolchains")

	return nil
}

func (r *Runner) installAllVersions(ctx context.Context) (map[goversion.GoVersion]string, error) {
	jobs := make(chan goversion.GoVersion, len(r.Versions))
	results := make(chan struct {
		version goversion.GoVersion
		exe     string
		err     error
	})

	cancellableContext, cancel := context.WithCancel(ctx)
	defer cancel()

	for i := 0; i < r.Parallelism; i++ {
		go func() {
			for j := range jobs {
				exe, err := r.installSingleVersion(cancellableContext, j)
				results <- struct {
					version goversion.GoVersion
					exe     string
					err     error
				}{j, exe, err}
			}
		}()
	}

	for _, job := range r.Versions {
		jobs <- job
	}
	close(jobs)

	exeLocations := make(map[goversion.GoVersion]string)
	for range r.Versions {
		select {
		case result := <-results:
			if result.err != nil {
				// Bail early and return
				cancel()
				return nil, result.err
			}
			exeLocations[result.version] = result.exe
			log.Printf("[%s--install] installed to %s", result.version, result.exe)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return exeLocations, nil
}

func (r *Runner) installSingleVersion(ctx context.Context, version goversion.GoVersion) (string, error) {
	installation := r.makeInstallation(version)
	goExe, errOutput, err := installation.Install(ctx)
	if err != nil {
		if errOutput != nil {
			// Dump the output of the subprocess
			scanner := bufio.NewScanner(bytes.NewReader(errOutput))
			for scanner.Scan() {
				log.Printf("[%s--install] [output] %s", version, scanner.Text())
			}
		}

		return "", fmt.Errorf("error while installing Go toolchain version %s: %w", version, err)
	}

	return goExe, nil
}

func (r *Runner) runSingleCommand(ctx context.Context, goExe string, version goversion.GoVersion, arch string) error {
	command := r.GetCommand(ctx, version, arch, goExe)
	if command == nil {
		// Allow the GetCommand implementation to skip a version/arch combination
		return nil
	}

	// Dump the command
	log.Printf("[%s-%s] %s %s %s", version, arch, strings.Join(command.Env, " "), command.Path, strings.Join(command.Args[1:], " "))

	output, err := command.CombinedOutput()
	if err != nil {
		// Dump the output of the subprocess
		scanner := bufio.NewScanner(bytes.NewReader(output))
		for scanner.Scan() {
			log.Printf("[%s-%s] [output] %s", version, arch, scanner.Text())
		}

		return fmt.Errorf("error while running command for (Go version, arch pair) (go%s, %s): %w", version, arch, err)
	}

	return nil
}

func (r *Runner) makeInstallation(version goversion.GoVersion) rungo.GoInstallation {
	return rungo.GoInstallation{
		Version:         version.String(),
		InstallGopath:   filepath.Join(r.InstallDirectory, "install-gopath"),
		InstallGocache:  filepath.Join(r.InstallDirectory, "install-gocache"),
		InstallLocation: r.getInstallLocation(),
	}
}

func (r *Runner) getInstallLocation() string {
	return filepath.Join(r.InstallDirectory, "install")
}

func getCombinations(versions []goversion.GoVersion, architectures []string) []architectureVersion {
	i := 0
	combinations := make([]architectureVersion, len(versions)*len(architectures))
	for _, v := range versions {
		for _, a := range architectures {
			combinations[i] = architectureVersion{architecture: a, version: v}
			i++
		}
	}

	return combinations
}
