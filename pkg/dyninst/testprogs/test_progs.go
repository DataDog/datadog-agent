// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package testprogs contains logic to build and use go programs for testing.
package testprogs

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"

	"github.com/gofrs/flock"
)

// CommonConfigs are some common configs that we can use for testing.
var CommonConfigs = []Config{
	{GOARCH: Amd64, GOTOOLCHAIN: CurrentVersion},
	{GOARCH: Arm64, GOTOOLCHAIN: CurrentVersion},
}

// GetBinary returns the path to the binary for the given name and metadata.
func GetBinary(
	name string,
	cfg Config,
) (string, error) {
	if err := cfg.Validate(); err != nil {
		return "", fmt.Errorf("invalid metadata: %w", err)
	}

	binaryDir := path.Join(binariesDir, cfg.String())
	binaryPath := path.Join(binaryDir, name)
	progDir := path.Join(progsDir, name)

	if err := os.MkdirAll(binaryDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create binary directory: %w", err)
	}
	{
		upToDate, err := checkIfUpToDate(progDir, binaryPath)
		if err != nil {
			return "", fmt.Errorf("failed to check if binary is up to date: %w", err)
		}
		if upToDate {
			log.Printf("binary %q is up to date", binaryPath)
			return binaryPath, nil
		}
	}

	fLock := flock.New(path.Join(binaryDir, flockName))
	if err := fLock.Lock(); err != nil {
		return "", fmt.Errorf("failed to lock flock: %w", err)
	}
	defer fLock.Close()
	{
		upToDate, err := checkIfUpToDate(progDir, binaryPath)
		if err != nil {
			return "", fmt.Errorf("failed to check if binary is up to date: %w", err)
		}
		if upToDate {
			log.Printf("binary %q is up to date", binaryPath)
			return binaryPath, nil
		}
	}

	if err := compileBinary(progDir, cfg, binaryPath); err != nil {
		return "", fmt.Errorf("failed to compile binary: %w", err)
	}

	return binaryPath, nil
}

func checkIfUpToDate(progDir string, binPath string) (bool, error) {
	binInfo, err := os.Stat(binPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to get binary info: %w", err)
	}
	binModTime := binInfo.ModTime()

	files, err := os.ReadDir(progDir)
	if err != nil {
		return false, fmt.Errorf("failed to read program directory: %w", err)
	}
	for _, file := range files {
		info, err := file.Info()
		if err != nil {
			return false, fmt.Errorf("failed to get file info: %w", err)
		}
		if info.ModTime().After(binModTime) {
			return false, nil
		}
	}
	return true, nil
}

// Config is the metadata for a test program.
type Config struct {
	GOARCH      string
	GOTOOLCHAIN string
}

// String returns a string representation of the config.
func (m *Config) String() string {
	return fmt.Sprintf("arch=%s,toolchain=%s", m.GOARCH, m.GOTOOLCHAIN)
}

// Go124 is the go version 1.24.1.
const Go124 = "go1.24.1"

// Local is the local go version.
const Local = "local"

const (
	// Amd64 is the amd64 architecture.
	Amd64 = "amd64"
	// Arm64 is the arm64 architecture.
	Arm64 = "arm64"
)

// Validate validates the metadata.
func (m *Config) Validate() error {
	switch m.GOARCH {
	case Amd64, Arm64:
	case "":
		return fmt.Errorf("GOARCH is required")
	default:
		return fmt.Errorf("GOARCH is invalid: %q", m.GOARCH)
	}

	if m.GOTOOLCHAIN == "" {
		return fmt.Errorf("GOTOOLCHAIN is required")
	}
	if !goVersionRegex.MatchString(m.GOTOOLCHAIN) {
		return fmt.Errorf("GOTOOLCHAIN is invalid: %q", m.GOTOOLCHAIN)
	}
	return nil
}

var (
	goVersionRegex = regexp.MustCompile(`^(go1\.\d+\.\d+|local)$`)
)

const flockName = ".flock"

// binariesDir is the directory where the binaries are stored.
var testProgsDir = func() string {
	_, file, _, ok := runtime.Caller(1)
	if !ok {
		panic("unable to get current file build path")
	}
	return filepath.Dir(file)
}()

// binariesDir is the directory where the binaries are stored.
var binariesDir = path.Join(testProgsDir, "binaries")
var progsDir = path.Join(testProgsDir, "progs")

// Packages is the list of packages that are available for testing.
var Packages = func() []string {
	files, err := os.ReadDir(progsDir)
	if err != nil {
		panic(err)
	}
	var names []string
	for _, file := range files {
		if !file.IsDir() {
			continue
		}
		names = append(names, file.Name())
	}
	return names
}()

// CurrentVersion is the current go version.
var CurrentVersion = func() string {
	curDir := testProgsDir
	goVersionRegex := regexp.MustCompile("\ngo (1\\.\\d+\\.\\d+)")
	for curDir != "." && curDir != "/" {
		goMod, err := os.ReadFile(path.Join(curDir, "go.mod"))
		if errors.Is(err, os.ErrNotExist) {
			curDir = filepath.Dir(curDir)
			continue
		}
		if err != nil {
			panic(err)
		}
		match := goVersionRegex.FindStringSubmatch(string(goMod))
		if len(match) == 0 {
			panic(fmt.Errorf("go.mod is invalid: %q", string(goMod)))
		}
		return "go" + match[1]
	}
	panic("go.mod not found")
}()

func compileBinary(progDir string, cfg Config, binPath string) error {
	log.Printf("compiling binary %q", binPath)

	cmd := exec.Command("go", "build", "-C", progDir, "-o", binPath, ".")
	cmd.Env = append(os.Environ(),
		"GOWORK=off",
		"GOOS=linux",
		"GOARCH="+cfg.GOARCH,
		"GOTOOLCHAIN="+cfg.GOTOOLCHAIN,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to compile binary: %w", err)
	}

	return nil
}
