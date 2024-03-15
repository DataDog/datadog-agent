// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package rungo provides tools to run the Go toolchain.
package rungo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

// GoInstallation configures a call to (*GoInstallation).Install,
// allowing callers to specify the installation/cache directories used,
// in addition to the actual toolchain version.
type GoInstallation struct {
	// Go version, formatted as "Major.Minor.Rev" if Rev is 0,
	// otherwise "Major.Minor"
	Version string
	// $GOPATH to use when invoking `go install`.
	// Determines where the wrapper toolchain binary is installed.
	InstallGopath string
	// $GOCACHE to use when invoking `go install`.
	// In other contexts, this is usually `$HOME/.cache/go-build`
	// if no custom cache has been set.
	InstallGocache string
	// Optional location to install the actual toolchain.
	// If this is not specified, it will be installed to `$HOME/sdk/goX.X.X/`
	InstallLocation string
}

// Install installs the "go" toolchain for the given Go version
// by using the existing tooling in golang.org/dl/goX.X.X.
//
// At a high level, this:
//   - runs `go install` using the host Go toolchain, installing a wrapper
//     program to `<i.InstallGopath>/bin`.
//   - runs `<wrapper> download`, which downloads and extracts the compiled Go
//     toolchain to `<i.InstallLocation>/sdk/goX.X.X`, or `$HOME/sdk/goX.X.X/`
//     if that is not specified
//
// If an error occurs, the function returns
// the error value in the third return value.
// If the error occurs while as the result of running a command,
// then its output is also returned in the second return value.
// Otherwise, if installation is successful,
// this function returns the absolute path to the binary
// in the first return value.
//
// This function can be called if the toolchain is already installed;
// in that case it will return early.
//
// Note: when using the Go binary that this function returns,
// the $HOME environment variable must be set to point to the same
// value as `i.InstallLocation` (if a custom value was given).
func (i *GoInstallation) Install(ctx context.Context) (string, []byte, error) {
	if i.Version == "" {
		return "", nil, fmt.Errorf("i.Version is required")
	}
	if i.InstallGopath == "" {
		return "", nil, fmt.Errorf("i.InstallGopath is required")
	}
	if i.InstallGocache == "" {
		return "", nil, fmt.Errorf("i.InstallGocache is required")
	}

	// Run the `go install` command to compile/install the wrapper binary
	// at `$GOPATH/bin/goX.X.X` (using the supplied `$GOPATH`)
	command := []string{"go", "install", fmt.Sprintf("golang.org/dl/go%s@latest", i.Version)}
	installCmd := exec.CommandContext(ctx, command[0], command[1:]...)
	installCmd.Env = append(installCmd.Env, fmt.Sprintf("%s=%s", "GOPATH", i.InstallGopath))
	installCmd.Env = append(installCmd.Env, fmt.Sprintf("%s=%s", "GOCACHE", i.InstallGocache))
	installOutput, err := installCmd.CombinedOutput()
	if err != nil {
		return "", installOutput, fmt.Errorf("error while running %s: %w", strings.Join(command, " "), err)
	}

	// Build the path to the wrapper binary, and make sure it exists
	wrapperPath := path.Join(i.InstallGopath, "bin", fmt.Sprintf("go%s", i.Version))
	absWrapperPath, err := filepath.Abs(wrapperPath)
	if err != nil {
		return "", nil, fmt.Errorf("error while resolving path to install wrapper binary (expected at %q): %w", wrapperPath, err)
	}
	if _, err := os.Stat(absWrapperPath); errors.Is(err, os.ErrNotExist) {
		return "", nil, fmt.Errorf("wrapper binary does not exist after install (expected at %q)", absWrapperPath)
	}

	// Run `<wrapper> download`,
	// and supply `i.InstallLocation` if given as a fake `$HOME`.
	// This is a hack, but the wrapper always installs to `<$HOME>/sdk/goX.X.X/`
	// https://cs.opensource.google/go/dl/+/faba4426:internal/version/version.go;l=417
	command = []string{absWrapperPath, "download"}
	downloadCmd := exec.CommandContext(ctx, command[0], command[1:]...)
	if i.InstallLocation != "" {
		downloadCmd.Env = append(downloadCmd.Env, fmt.Sprintf("%s=%s", "HOME", i.InstallLocation))
	}
	downloadOutput, err := downloadCmd.CombinedOutput()
	if err != nil {
		return "", downloadOutput, fmt.Errorf("error while running %s: %w", strings.Join(command, " "), err)
	}

	return absWrapperPath, nil, nil
}
