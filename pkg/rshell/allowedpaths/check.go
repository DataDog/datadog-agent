// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package allowedpaths

import (
	"fmt"
	"os"
)

// PathOperation identifies a policy check, without performing the operation.
type PathOperation string

const (
	PathRead   PathOperation = "read"
	PathStat   PathOperation = "stat"
	PathLstat  PathOperation = "lstat"
	PathWrite  PathOperation = "write"
	PathRemove PathOperation = "remove"
)

// CheckPath checks the current sandbox policy using metadata only. It never
// opens the target for reading or writing, creates it, or removes it. Missing
// files are permitted: this checks authorization, not existence or OS access
// rights. Symlinks, root access modes, and existing write-target hard links are
// checked using the same helpers as execution. The result is advisory; every
// operation must still enforce policy at the time it is performed.
func (s *Sandbox) CheckPath(path, cwd string, operation PathOperation) error {
	if (operation == PathStat || operation == PathLstat) && IsDevNull(path) {
		return nil // Stat and Lstat expose null-device metadata without a root.
	}
	abs := toAbs(path, cwd)
	denied := func() error {
		return &os.PathError{Op: string(operation), Path: path, Err: os.ErrPermission}
	}
	if s == nil {
		return denied()
	}
	if operation == PathRead || operation == PathStat || operation == PathLstat {
		ar, rel, ok := s.resolve(abs)
		if !ok {
			return denied()
		}
		// Match Open's primary rooted operation before its cross-root fallback.
		// Walking every symlink ourselves would incorrectly apply the fallback's
		// smaller hop limit to ordinary in-root symlink chains.
		var err error
		if operation == PathLstat {
			_, err = ar.root.Lstat(rel)
		} else {
			_, err = ar.root.Stat(rel)
		}
		if isPathEscapeError(err) {
			if _, _, ok := s.resolveRootFollowingSymlinks(abs, operation == PathLstat); !ok {
				return denied()
			}
		}
		return nil
	}
	if operation != PathWrite && operation != PathRemove {
		return fmt.Errorf("unknown path operation %q", operation)
	}
	if s.readOnly {
		return denied()
	}
	var ar *root
	var rel string
	var ok bool
	if operation == PathRemove {
		ar, rel, ok = s.resolveRemoveTarget(abs)
	} else {
		ar, rel, ok = s.resolveWriteTarget(abs)
	}
	if !ok {
		return denied()
	}
	if err := ar.rejectSymlinkPathComponents(rel, operation == PathWrite); err != nil {
		return rewrapPathError(string(operation), path, err)
	}
	info, err := ar.root.Lstat(rel)
	if err != nil {
		// Missing or inaccessible metadata does not establish a policy denial.
		// Execution is responsible for existence and OS permission errors.
		return nil
	}
	if operation == PathRemove {
		if info.IsDir() {
			return fmt.Errorf("remove %s: directories are not supported", path)
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("write %s: not a regular file", path)
	}
	if links, known := fileLinkCount(info); known && links > 1 {
		return &os.PathError{Op: "write", Path: path, Err: ErrMultiplyLinkedWriteTarget}
	}
	return nil
}
