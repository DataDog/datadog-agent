// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package extract

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/sassoftware/go-rpmutils"

	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/types"
)

// ExtractRPMPackage extracts an RPM from reader into directory for kernelUname.
func ExtractRPMPackage(reader io.Reader, directory, kernelUname string, logger types.Logger) error {
	header, err := rpmutils.ReadHeader(reader)
	if err != nil {
		return fmt.Errorf("parse RPM package: %w", err)
	}
	destination, err := openExtractionDirectory(directory)
	if err != nil {
		return err
	}
	defer destination.root.Close()
	payload, err := openRPMPayload(reader, header)
	if err != nil {
		return fmt.Errorf("open RPM payload: %w", err)
	}
	if err := extractRPMPayload(payload, destination); err != nil {
		return fmt.Errorf("extract RPM package: %w", err)
	}
	return fixKernelModulesSymlinks(destination, kernelUname, logger)
}

// fixKernelModulesSymlinks repairs lib/modules/<release>/{build,source}, whose
// installation targets describe /usr/src on the host rather than the downloaded
// cache. Absolute installation targets become relative links into the cache;
// relative targets keep their installation meaning. Older cache-prefixed
// absolute targets are recognized too, so repair can run repeatedly.
// All accesses stay under os.Root, and symlinked parent directories are rejected
// before a link is removed or replaced. Repair failures are logged and returned
// so the caller discards the incomplete download instead of reusing it.
func fixKernelModulesSymlinks(destination *extractionDirectory, kernelUname string, logger types.Logger) error {
	if !filepath.IsLocal(kernelUname) || strings.Contains(kernelUname, "/") || kernelUname == "." {
		return fmt.Errorf("invalid kernel release %q", kernelUname)
	}

	for _, linkName := range []string{"build", "source"} {
		name := filepath.Join("lib/modules", kernelUname, linkName)
		if err := repairModuleSymlink(destination, name, logger); err != nil {
			if logger != nil {
				logger.Warnf("repair module symlink %s: %v", name, err)
			}
			return fmt.Errorf("repair module symlink %s: %w", name, err)
		}
	}
	return nil
}

// repairModuleSymlink adjusts one extracted build or source link to resolve
// inside the header directory. RPMs may use absolute installation paths such
// as /usr/src/kernels/<release>; those become relative links into this tree.
// Relative links already have the right meaning, and older cache-absolute
// links are recognized so repair can run again on an existing extraction.
func repairModuleSymlink(destination *extractionDirectory, name string, logger types.Logger) error {
	info, err := destination.root.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return nil
	}

	target, err := destination.root.Readlink(name)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(target) {
		return destination.symlink(name, target)
	}

	// Legacy cache links already include the destination. Check path components
	// so a neighboring directory sharing the cache's string prefix is not matched.
	if relative, err := filepath.Rel(destination.root.Name(), target); err == nil && filepath.IsLocal(relative) {
		target = "/" + relative
	}
	// Validate and relocate the target before removing the original link.
	target, err = linkTarget(name, target)
	if err != nil {
		return err
	}
	if err := destination.mkdir(filepath.Dir(name)); err != nil {
		return err
	}
	if err := destination.root.Remove(name); err != nil {
		return err
	}
	if err := destination.symlink(name, target); err != nil {
		return err
	}
	if logger != nil {
		logger.Debugf("created module symlink from %s to %s", name, target)
	}
	return nil
}
