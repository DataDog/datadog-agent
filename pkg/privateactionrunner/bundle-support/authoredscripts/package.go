// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package authoredscripts

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

// Package is a cached authored-script package that is ready to be executed.
type Package struct {
	// Manifest describes the script, the command that runs it, and the environment it
	// expects.
	Manifest *Manifest
	// Directory is the absolute path of the package in the artifact cache.
	Directory string
	// ArtifactDigest is the sha256 digest of the artifact the package was extracted
	// from, identifying exactly which build of the script this is.
	ArtifactDigest string
	// Command is the command to execute, with its first element resolved to an absolute
	// path inside Directory and its arguments left as the manifest declared them.
	Command []string
	// ToolDirectories holds the absolute directory of each tool bundled with the
	// package, in the order the manifest declares them.
	ToolDirectories []string
}

// Store opens authored-script packages for execution.
type Store interface {
	Open(fqn string) (*Package, error)
}

// localStore opens packages from the local artifact cache. It never downloads, so an
// action whose package has not been cached yet fails with ErrNotCached.
type localStore struct {
	cacheRoot string
	platform  string
	resolver  Resolver
}

// NewLocalStore returns a Store that opens packages cached under cacheRoot, using
// resolver to choose which artifact to open for an action.
func NewLocalStore(cacheRoot string, resolver Resolver) Store {
	return &localStore{cacheRoot: cacheRoot, platform: currentPlatform(), resolver: resolver}
}

// Open resolves an action to its cached package and validates everything that package
// needs in order to run. Validating up front means a package that cannot be executed is
// reported before an execution starts, rather than failing part way through it where the
// cause is much harder to attribute.
//
// The artifact's digest is not recomputed here. Digests are verified when a package is
// downloaded, and published packages are immutable, so a package that carries the
// completion marker has already been verified.
func (s *localStore) Open(fqn string) (*Package, error) {
	if err := validateFQN(fqn); err != nil {
		return nil, err
	}

	digest, err := s.resolver.Resolve(fqn)
	if err != nil {
		return nil, err
	}
	// The resolver is an interface, so the digest it returns is validated before it is
	// used to build a path.
	if err := validateDigest(digest); err != nil {
		return nil, fmt.Errorf("could not resolve authored-script action %s: %w", fqn, err)
	}

	directory := packageDirectory(s.cacheRoot, fqn, digest, s.platform)
	if err := checkComplete(directory); err != nil {
		return nil, fmt.Errorf("could not open authored-script package for %s: %w", fqn, err)
	}

	manifest, err := LoadManifest(directory)
	if err != nil {
		return nil, err
	}
	if manifest.FQN != fqn {
		return nil, fmt.Errorf("cached authored-script package for %s declares action %q", fqn, manifest.FQN)
	}

	command, err := resolveCommand(directory, manifest.Config.Command)
	if err != nil {
		return nil, err
	}
	toolDirectories, err := resolveToolDirectories(directory, manifest.Dependencies)
	if err != nil {
		return nil, err
	}

	return &Package{
		Manifest:        manifest,
		Directory:       directory,
		ArtifactDigest:  digest,
		Command:         command,
		ToolDirectories: toolDirectories,
	}, nil
}

// checkComplete reports whether a package directory carries the marker written once the
// package has been fully extracted and verified. A directory without it is treated as
// absent, so that a download interrupted part way through never looks like a cache hit.
func checkComplete(directory string) error {
	info, err := os.Stat(filepath.Join(directory, completionMarker))
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: the %s marker is missing", ErrNotCached, completionMarker)
	}
	if err != nil {
		return fmt.Errorf("could not read the %s marker: %w", completionMarker, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("the %s marker is not a regular file", completionMarker)
	}
	return nil
}

// resolveCommand returns the manifest's command with its executable resolved to an
// absolute path inside the package, after checking that the executable is there and can
// actually be run.
func resolveCommand(directory string, command []string) ([]string, error) {
	// filepath.Join would quietly reinterpret an absolute path as a package-relative
	// one, so reject it rather than running something other than what was declared.
	if filepath.IsAbs(command[0]) {
		return nil, fmt.Errorf("authored-script command %q must be relative to the package", command[0])
	}

	executable := filepath.Join(scriptDirectory, command[0])
	file, err := openPackageFile(directory, executable)
	if err != nil {
		return nil, fmt.Errorf("could not open authored-script command: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("could not read authored-script command: %w", err)
	}
	if err := checkExecutable(info); err != nil {
		return nil, fmt.Errorf("authored-script command %q cannot be executed: %w", command[0], err)
	}

	resolved := make([]string, len(command))
	copy(resolved, command)
	resolved[0] = filepath.Join(directory, executable)
	return resolved, nil
}

// checkExecutable reports whether a file can be executed. Windows carries no executable
// bit, so there is nothing to check there.
func checkExecutable(info fs.FileInfo) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("file mode is %s", info.Mode().Perm())
	}
	return nil
}

// isDirectoryEntryName reports whether name identifies a single entry inside a
// directory, rather than a path leading somewhere else. A tool is one directory inside
// tools/, so anything else would resolve outside the layout: ".." and absolute paths
// escape it, "a/b" reaches past it, and "." and "" name the tools directory itself.
func isDirectoryEntryName(name string) bool {
	return filepath.IsLocal(name) && name == filepath.Base(name) && name != "."
}

// resolveToolDirectories returns the absolute directory of every tool the manifest
// declares as a dependency. Missing tools are reported here because a script that only
// discovers a missing tool once it is running fails in ways that are hard to attribute.
func resolveToolDirectories(directory string, dependencies []Dependency) ([]string, error) {
	if len(dependencies) == 0 {
		return nil, nil
	}

	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, fmt.Errorf("could not open authored-script package: %w", err)
	}
	defer root.Close()

	directories := make([]string, 0, len(dependencies))
	for _, dependency := range dependencies {
		if !isDirectoryEntryName(dependency.Name) {
			return nil, fmt.Errorf("authored-script tool %q is not a valid tool name", dependency.Name)
		}

		relative := filepath.Join(toolsDirectory, dependency.Name)
		info, err := root.Stat(relative)
		if err != nil {
			return nil, fmt.Errorf("could not access authored-script tool %q: %w", dependency.Name, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("authored-script tool %q is not a directory", dependency.Name)
		}
		directories = append(directories, filepath.Join(directory, relative))
	}
	return directories, nil
}
