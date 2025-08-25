// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package testprogs contains logic to build and use go programs for testing.
//
// The package relies on the binaries being built in the `binaries` directory
// and the source code being available in the `progs` directory. The binaries
// are built by the `system_probe.py` invoke script.
//
// The package will check the local sources to determine if the binaries are
// up to date, but it won't rebuild them if they are not. The hope is that the
// binaries we build will be sufficiently reproducible that we can use them for
// testing.
package testprogs

import (
	"cmp"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

const helpMsg = "consider running `dda inv system-probe.build-dyninst-test-programs`"

// MustGetCommonConfigs calls GetCommonConfigs and checks for an error..
func MustGetCommonConfigs(t testing.TB) []Config {
	cfgs, err := GetCommonConfigs()
	require.NoError(t, err)
	return cfgs
}

// MustGetPrograms calls GetPrograms and checks for an error.
func MustGetPrograms(t testing.TB) []string {
	programs, err := GetPrograms()
	require.NoError(t, err)
	return programs
}

// MustGetBinary calls GetBinary and checks for an error.
func MustGetBinary(t testing.TB, name string, cfg Config) string {
	bin, err := GetBinary(name, cfg)
	require.NoError(t, err)
	return bin
}

// GetCommonConfigs returns a list of configurations that are suggested for
// use in tests. In scenarios where the source code is available, other
// configurations may still be available via GetBinary.
func GetCommonConfigs() ([]Config, error) {
	state, err := getState()
	if err != nil {
		return nil, fmt.Errorf("testprogs: %w", err)
	}
	return state.commonConfigs, nil
}

// GetPrograms returns a list of programs that are available for testing.
func GetPrograms() ([]string, error) {
	state, err := getState()
	if err != nil {
		return nil, fmt.Errorf("testprogs: %w", err)
	}
	return state.programs, nil
}

// GetBinary returns the path to the binary for the given name and
// configuration. If the binary is not found, it will be compiled if the source
// code is available.
func GetBinary(name string, cfg Config) (string, error) {
	state, err := getState()
	if err != nil {
		return "", fmt.Errorf("testprogs: %w", err)
	}
	bin, err := getBinary(state, name, cfg)
	if err != nil {
		return "", fmt.Errorf("testprogs: %w", err)
	}
	return bin, nil
}

// state is the state of the testprogs package.
type state struct {
	// A list of common configurations that are available for testing.
	commonConfigs []Config
	// A list of programs that are available for testing.
	programs []string
	// The directory where the binaries are stored.
	binariesDir string
	// The directory where the source code is stored, may be empty if the
	// source code is not available.
	progsSrcDir string
	// Whether the source code is available.
	haveSources bool
	// The directory where the probe configs are stored.
	probesCfgsDir string
}

var (
	globalState     state
	globalStateErr  error
	globalStateOnce sync.Once
)

// getState returns the global state of the testprogs package.
func getState() (*state, error) {
	globalStateOnce.Do(func() {
		var haveSources bool
		var progsSrcDir string
		if _, srcPath, _, ok := runtime.Caller(0); ok {
			srcDir := path.Dir(srcPath)
			s, err := os.Stat(srcDir)
			haveSources = err == nil && s.IsDir()
			if haveSources {
				progsSrcDir = path.Join(srcDir, "progs")
			}
		}
		globalState, globalStateErr = initStateFromBinaries(
			haveSources,
			progsSrcDir,
		)
	})
	return &globalState, globalStateErr
}

func initStateFromBinaries(
	haveSources bool,
	progsSrcDir string,
) (state, error) {
	pkgPath := strings.TrimPrefix(
		reflect.TypeOf(Config{}).PkgPath(),
		"github.com/DataDog/datadog-agent/",
	)
	const maxDirectoryDepth = 10
	binariesDir := path.Join(".", pkgPath, "binaries")
	for range maxDirectoryDepth {
		if _, err := os.Stat(binariesDir); err == nil {
			goto found
		}
		binariesDir = path.Join("..", binariesDir)
	}
	return state{}, fmt.Errorf("binaries directory not found; %s", helpMsg)
found:
	binariesDir, err := filepath.Abs(binariesDir)
	if err != nil {
		return state{}, fmt.Errorf("failed to get absolute path for binaries directory: %w", err)
	}
	probesCfgsDir, err := filepath.Abs(path.Join(binariesDir, "../testdata/probes"))
	if err != nil {
		return state{}, fmt.Errorf("failed to get absolute path for probes directory: %w", err)
	}
	// Now we want to iterate over the binaries directory and read the
	// packages names of the directories as well as parsing out the
	// configuration from the directory name.
	programConfigs := map[string]int{}
	configs := map[Config]struct{}{}
	files, err := os.ReadDir(binariesDir)
	if err != nil {
		return state{}, fmt.Errorf("failed to read binaries directory: %w", err)
	}
	for _, file := range files {
		if !file.IsDir() {
			continue
		}
		cfg, err := parseConfig(file.Name())
		if err != nil {
			return state{}, fmt.Errorf("failed to parse config from directory name: %w", err)
		}
		files, err := os.ReadDir(path.Join(binariesDir, file.Name()))
		if err != nil {
			return state{}, fmt.Errorf("failed to read program directory: %w", err)
		}
		for _, file := range files {
			name := file.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			info, err := file.Info()
			if err != nil {
				return state{}, fmt.Errorf("failed to get file info: %w", err)
			}
			if !info.Mode().IsRegular() {
				continue
			}
			programConfigs[file.Name()]++
			// Only count the config if there's at least one program for it.
			configs[cfg] = struct{}{}
		}
	}
	numConfigs := len(configs)
	programs := make([]string, 0, len(programConfigs))
	for name := range programConfigs {
		if programConfigs[name] == numConfigs {
			programs = append(programs, name)
		}
	}
	commonConfigs := make([]Config, 0, len(configs))
	for cfg := range configs {
		commonConfigs = append(commonConfigs, cfg)
	}
	slices.SortFunc(commonConfigs, func(a, b Config) int {
		return cmp.Or(
			cmp.Compare(a.GOARCH, b.GOARCH),
			cmp.Compare(a.GOTOOLCHAIN, b.GOTOOLCHAIN),
		)
	})

	return state{
		commonConfigs: commonConfigs,
		programs:      programs,
		binariesDir:   binariesDir,
		progsSrcDir:   progsSrcDir,
		haveSources:   haveSources,
		probesCfgsDir: probesCfgsDir,
	}, nil
}

// getBinary returns the path to the binary for the given name and metadata.
func getBinary(
	state *state,
	name string,
	cfg Config,
) (string, error) {
	if err := cfg.Validate(); err != nil {
		return "", fmt.Errorf("invalid metadata: %w", err)
	}

	binariesDir := state.binariesDir
	progsSrcDir := state.progsSrcDir
	binaryDir := path.Join(binariesDir, cfg.String())
	binaryPath := path.Join(binaryDir, name)
	progDir := path.Join(progsSrcDir, name)
	binInfo, err := os.Stat(binaryPath)
	if errors.Is(err, os.ErrNotExist) {
		log.Printf(
			"binary %q with config %q does not exist; %s",
			name, cfg.String(),
			helpMsg,
		)
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf(
			"failed to get binary info for %q with config %q: %w",
			name, cfg.String(), err,
		)
	}

	if state.haveSources {
		upToDate, err := checkIfUpToDate(progDir, binInfo)
		if err != nil {
			return "", fmt.Errorf(
				"failed to check if binary %q is up to date: %w", name, err,
			)
		}
		if !upToDate {
			log.Printf(
				"NOTE: binary %q with config %q is not up to date; %s",
				name, cfg.String(),
				helpMsg,
			)
		}
	}

	return binaryPath, nil
}

func checkIfUpToDate(progDir string, binInfo os.FileInfo) (bool, error) {
	binModTime := binInfo.ModTime()
	files, err := os.ReadDir(progDir)
	if err != nil {
		return false, fmt.Errorf("failed to read program directory %q: %w", progDir, err)
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
//
// Note that this format corresponds to the format used by the code in
// tasks/system_probe.py that builds the binaries.
func (m *Config) String() string {
	return fmt.Sprintf("arch=%s,toolchain=%s", m.GOARCH, m.GOTOOLCHAIN)
}

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

func parseConfig(s string) (Config, error) {
	parts := strings.Split(s, ",")
	var cfg Config
	for _, part := range parts {
		parts := strings.Index(part, "=")
		if parts == -1 {
			return Config{}, fmt.Errorf("invalid config: %q", s)
		}
		switch part[:parts] {
		case "arch":
			cfg.GOARCH = part[parts+1:]
		case "toolchain":
			cfg.GOTOOLCHAIN = part[parts+1:]
		default:
			return Config{}, fmt.Errorf("invalid config: %q", s)
		}
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("invalid config: %w", err)
	}
	return cfg, nil
}

var (
	goVersionRegex = regexp.MustCompile(`^(go1\.\d+\.\d+)$`)
)
