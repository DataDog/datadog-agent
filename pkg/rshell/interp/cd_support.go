// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"mvdan.cc/sh/v3/expand"
)

// errNoSandbox is returned by changeDir when no AllowedPaths sandbox has
// been configured. With no sandbox, all file access is blocked, so cd
// has nowhere safe to land.
var errNoSandbox = errors.New("no allowed paths configured")

// errInvalidPath is returned when changeDir is called with a relative
// path. The cd builtin always passes absolute paths; a relative path
// here would indicate a programming error in the caller.
var errInvalidPath = errors.New("path must be absolute")

// errNotDirectory is returned when the target exists but is not a
// directory.
var errNotDirectory = errors.New("not a directory")

// changeDir mutates the runner's working directory after validating the
// supplied path. It is exposed to the cd builtin via CallContext.ChangeDir.
//
// absDir must be absolute. The directory must exist, be a directory, and
// resolve inside an AllowedPaths root. On success, $OLDPWD is set to the
// previous working directory and $PWD is set to absDir.
//
// On any failure (path not absolute, no sandbox, target missing or not a
// directory, target outside the sandbox) the working directory is left
// unchanged and an error is returned.
func (r *Runner) changeDir(absDir string) error {
	if !filepath.IsAbs(absDir) {
		return &os.PathError{Op: "chdir", Path: absDir, Err: errInvalidPath}
	}
	if r.sandbox == nil {
		return &os.PathError{Op: "chdir", Path: absDir, Err: errNoSandbox}
	}
	cleaned := filepath.Clean(absDir)
	info, err := r.sandbox.Stat(cleaned, r.Dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return &os.PathError{Op: "chdir", Path: absDir, Err: errNotDirectory}
	}
	// bash's cd refuses to enter a directory the user lacks search
	// (execute) permission on. Stat alone does not check this, so verify
	// access through the sandbox before committing the change. The
	// 0x01 mode is the same execute bit accepted by AccessFile callers.
	//
	// Skipped on Windows: there are no POSIX execute bits, and
	// allowedpaths.accessCheck always denies an execute check on Windows.
	// Directory traversal is governed by ACLs that os.Root has already
	// honoured at sandbox open, so the Stat above is a sufficient guard.
	if runtime.GOOS != "windows" {
		if err := r.sandbox.Access(cleaned, r.Dir, 0x01); err != nil {
			return err
		}
	}

	// bash's OLDPWD source is the **env var** $PWD, not the shell's
	// tracked working directory. The two normally agree, but they can
	// differ when a temporary assignment precedes cd
	// (e.g. `PWD=/bogus cd b`) — bash captures /bogus as OLDPWD even
	// though the actual cwd was /tmp/parent. Fall back to r.Dir only
	// when $PWD is unset.
	oldDir := r.Dir
	if v := r.writeEnv.Get("PWD"); v.IsSet() && v.Str != "" {
		oldDir = v.Str
	}

	// Apply env writes BEFORE committing r.Dir so a failed cd leaves
	// shell state untouched (matches bash, which never mutates pwd on a
	// failed cd). setVarErr can fail when the total variable storage
	// cap is exhausted; if either write errors, restore any prior
	// OLDPWD and return without touching r.Dir. Without this ordering
	// a failed env write would leave subsequent relative file
	// operations resolving from a new directory while $PWD/$OLDPWD
	// still pointed elsewhere.
	prevOLDPWD := r.writeEnv.Get("OLDPWD")
	if err := r.setVarErr("OLDPWD", expand.Variable{Set: true, Kind: expand.String, Str: oldDir}); err != nil {
		return err
	}
	if err := r.setVarErr("PWD", expand.Variable{Set: true, Kind: expand.String, Str: cleaned}); err != nil {
		// Roll back OLDPWD to its pre-cd value via setVarRestore so
		// the storage cap doesn't block the restore itself.
		r.setVarRestore("OLDPWD", prevOLDPWD)
		return err
	}

	r.Dir = cleaned
	return nil
}

// lookupEnvVar returns the value of an environment variable in the
// runner's overlay environment, or ("", false) if unset. Used by the cd
// builtin to read $HOME and $OLDPWD without exposing the full
// WriteEnviron handle to every CallContext.
func (r *Runner) lookupEnvVar(name string) (string, bool) {
	vr := r.writeEnv.Get(name)
	if !vr.IsSet() {
		return "", false
	}
	return vr.Str, true
}
