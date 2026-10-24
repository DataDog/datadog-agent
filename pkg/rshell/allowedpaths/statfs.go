// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package allowedpaths

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/rshell/allowedpaths/internal/fsstat"
	"github.com/DataDog/datadog-agent/pkg/rshell/allowedpaths/internal/writeopen"
)

// FileSystemInfo is the normalized filesystem metadata returned by StatFS.
type FileSystemInfo = fsstat.Info

// StatFS returns filesystem metadata for path after resolving it through the
// AllowedPaths sandbox. The platform backend queries a rooted handle tied to
// the target filesystem and revalidates the target identity, so the result
// cannot be redirected outside the sandbox after validation.
func (s *Sandbox) StatFS(path, cwd string) (FileSystemInfo, error) {
	if path == "" {
		return FileSystemInfo{}, &os.PathError{Op: "statfs", Path: path, Err: os.ErrNotExist}
	}
	requireDirectory := writeopen.HasTrailingDirSyntax(path)

	for range maxSymlinkHops + 1 {
		resolver := statFSPathResolver{sandbox: s}
		absPath, canonicalPath, err := resolver.resolveParentComponents(path, cwd, false)
		if errors.Is(err, fsstat.ErrPathChanged) || isPathEscapeError(err) {
			continue
		}
		if err != nil {
			if errors.Is(err, fsstat.ErrNotSupported) {
				return FileSystemInfo{}, err
			}
			return FileSystemInfo{}, rewrapPathError("statfs", path, err)
		}

		ar, relPath, resolvedRequireDirectory, err := resolver.resolveRootFollowingSymlinks(
			absPath,
			canonicalPath,
			requireDirectory,
		)
		if errors.Is(err, fsstat.ErrPathChanged) || isPathEscapeError(err) {
			continue
		}
		if err != nil {
			return FileSystemInfo{}, rewrapPathError("statfs", path, err)
		}

		info, err := fsstat.Read(ar.root, relPath, resolvedRequireDirectory)
		if errors.Is(err, fsstat.ErrPathChanged) || isPathEscapeError(err) {
			continue
		}
		if err != nil {
			if errors.Is(err, fsstat.ErrNotSupported) {
				return FileSystemInfo{}, err
			}
			return FileSystemInfo{}, rewrapPathError("statfs", path, err)
		}
		return info, nil
	}

	return FileSystemInfo{}, &os.PathError{Op: "statfs", Path: path, Err: fsstat.ErrPathChanged}
}

type statFSPathResolver struct {
	sandbox     *Sandbox
	symlinkHops int
}

// resolveParentComponents preserves path-resolution semantics for
// operands containing "..". filepath.Join and filepath.Clean would otherwise
// erase the preceding component before the rooted backend can verify that it
// is a directory. Resolving each such prefix also makes ".." apply to a
// symlink's target directory rather than the link's lexical parent.
func (r *statFSPathResolver) resolveParentComponents(
	path, cwd string,
	canonicalPath bool,
) (string, bool, error) {
	current := filepath.Clean(cwd)
	componentPath := path
	if filepath.IsAbs(path) {
		volume := filepath.VolumeName(path)
		current = volume + string(filepath.Separator)
		componentPath = path[len(volume):]
	} else if len(path) > 0 && os.IsPathSeparator(path[0]) {
		current = filepath.VolumeName(cwd) + string(filepath.Separator)
	}

	for _, component := range splitStatFSPath(componentPath) {
		switch component {
		case ".":
			continue
		case "..":
			if filepath.Dir(current) == current {
				continue
			}
			ar, relPath, _, err := r.resolveRootFollowingSymlinks(current, canonicalPath, true)
			if err != nil {
				return "", false, err
			}
			if _, err := fsstat.Read(ar.root, relPath, true); err != nil {
				return "", false, err
			}
			current = filepath.Dir(filepath.Join(ar.canonicalAbsPath, relPath))
			canonicalPath = true
		default:
			current = filepath.Join(current, component)
		}
	}
	return current, canonicalPath, nil
}

func (r *statFSPathResolver) resolveRootFollowingSymlinks(
	absPath string,
	canonicalPath bool,
	requireDirectory bool,
) (*root, string, bool, error) {
	for {
		lookupPath := filepath.Clean(absPath)
		if canonicalPath {
			if ar, relPath, ok := r.sandbox.resolveCanonical(lookupPath); ok {
				lookupPath = filepath.Join(ar.absPath, relPath)
			}
		}

		ar, relPath, ok := r.sandbox.resolve(lookupPath)
		if !ok {
			return nil, "", false, os.ErrPermission
		}

		components := strings.Split(relPath, string(filepath.Separator))
		symlinkFound := false
		for i := range components {
			if components[i] == "." {
				continue
			}
			partial := strings.Join(components[:i+1], string(filepath.Separator))
			info, err := ar.root.Lstat(partial)
			if err != nil {
				// Let the identity-checked backend return the exact missing,
				// inaccessible, or non-directory error for the full path.
				return ar, relPath, requireDirectory, nil
			}
			if info.Mode()&fs.ModeSymlink == 0 {
				if i < len(components)-1 && !info.IsDir() {
					return nil, "", false, fsstat.ErrNotDirectory
				}
				continue
			}

			r.symlinkHops++
			if r.symlinkHops > maxSymlinkHops {
				return nil, "", false, os.ErrPermission
			}

			target, err := ar.root.Readlink(partial)
			if err != nil {
				return nil, "", false, os.ErrPermission
			}

			targetCanonical := false
			if filepath.IsAbs(target) {
				if r.sandbox.hostPrefix != "" &&
					target != r.sandbox.hostPrefix &&
					!strings.HasPrefix(target, r.sandbox.hostPrefix+string(filepath.Separator)) {
					target = r.sandbox.hostPrefix + target
				}
			} else if len(target) > 0 && os.IsPathSeparator(target[0]) {
				target = filepath.VolumeName(ar.canonicalAbsPath) + target
				targetCanonical = true
			} else {
				target = joinStatFSPath(
					filepath.Join(ar.canonicalAbsPath, filepath.Dir(partial)),
					target,
				)
				targetCanonical = true
			}

			if i < len(components)-1 {
				remaining := strings.Join(components[i+1:], string(filepath.Separator))
				target = joinStatFSPath(target, remaining)
			} else if writeopen.HasTrailingDirSyntax(target) {
				requireDirectory = true
			}

			absPath, canonicalPath, err = r.resolveParentComponents(target, "", targetCanonical)
			if err != nil {
				return nil, "", false, err
			}
			symlinkFound = true
			break
		}
		if !symlinkFound {
			return ar, relPath, requireDirectory, nil
		}
	}
}

func splitStatFSPath(path string) []string {
	var components []string
	start := 0
	for i := range len(path) {
		if !os.IsPathSeparator(path[i]) {
			continue
		}
		if start < i {
			components = append(components, path[start:i])
		}
		start = i + 1
	}
	if start < len(path) {
		components = append(components, path[start:])
	}
	return components
}

func joinStatFSPath(base, path string) string {
	if base == "" || path == "" {
		return base + path
	}
	if os.IsPathSeparator(base[len(base)-1]) {
		return base + path
	}
	return base + string(filepath.Separator) + path
}
