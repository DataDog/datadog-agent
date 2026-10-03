// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package config

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

var legacyManagedLinkPaths = []string{
	"managed/datadog-agent/stable",
	"managed/datadog-agent/experiment",
}

func isManagedConfigYAML(relativePath string) bool {
	normalizedPath := "/" + filepath.ToSlash(relativePath)
	if !strings.EqualFold(path.Ext(normalizedPath), ".yaml") {
		return false
	}

	legacyConfigPath := "/" + filepath.ToSlash(legacyPathPrefix)
	if normalizedPath == legacyConfigPath || strings.HasPrefix(normalizedPath, legacyConfigPath+"/") {
		return true
	}

	for _, spec := range allowedConfigFiles {
		matched, err := path.Match(spec.pattern, normalizedPath)
		if err == nil && matched {
			return true
		}
	}
	return false
}

func removeConfigFilesMissingFromSource(sourcePath, targetPath string) error {
	sourceRoot, err := os.OpenRoot(sourcePath)
	if err != nil {
		return fmt.Errorf("could not open source config directory: %w", err)
	}
	defer sourceRoot.Close()

	targetRoot, err := os.OpenRoot(targetPath)
	if err != nil {
		return fmt.Errorf("could not open target config directory: %w", err)
	}
	defer targetRoot.Close()

	emptiedDirs := make(map[string]struct{})
	err = walkFiles(targetRoot.FS(), ".", func(relativePath string, _ fs.DirEntry) error {
		if !isManagedConfigYAML(relativePath) {
			return nil
		}
		removed, err := removeConfigFileMissingFromSource(sourceRoot, targetRoot, relativePath)
		if err != nil {
			return err
		}
		if removed {
			if dir := path.Dir(relativePath); dir != "." {
				emptiedDirs[dir] = struct{}{}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	pruneEmptyManagedDirs(sourceRoot, targetRoot, emptiedDirs)
	return nil
}

// removeConfigFileMissingFromSource removes relativePath from the target when the source
// does not have it. It reports whether the file was removed.
func removeConfigFileMissingFromSource(sourceRoot, targetRoot *os.Root, relativePath string) (bool, error) {
	_, err := sourceRoot.Lstat(relativePath)
	if err == nil {
		return false, nil
	}
	if !os.IsNotExist(err) {
		return false, fmt.Errorf("could not check source config file %q: %w", relativePath, err)
	}
	if err := targetRoot.Remove(relativePath); err != nil {
		return false, fmt.Errorf("could not remove config file %q during rollback: %w", relativePath, err)
	}
	return true, nil
}

// pruneEmptyManagedDirs removes directories that cleanup emptied and the source lacks.
// Directories containing unmanaged files are preserved.
// Pruning is best effort so it cannot fail an otherwise successful rollback.
func pruneEmptyManagedDirs(sourceRoot, targetRoot *os.Root, emptiedDirs map[string]struct{}) {
	for _, dir := range dirsDeepestFirst(emptiedDirs) {
		if _, err := sourceRoot.Lstat(dir); err == nil || !os.IsNotExist(err) {
			// The source still has this directory, or we cannot tell: keep it.
			continue
		}
		if !isEmptyDir(targetRoot, dir) {
			continue
		}
		// A directory that cannot be removed is left in place. Its parents cannot be
		// empty either, so the rest of the pass skips them on its own.
		_ = targetRoot.Remove(dir)
	}
}

// dirsDeepestFirst expands dirs with their parent directories and orders them so that a
// directory always comes before its parent, letting a parent that is emptied by the removal
// of its last child be pruned in the same pass.
func dirsDeepestFirst(dirs map[string]struct{}) []string {
	all := make(map[string]struct{}, len(dirs))
	for dir := range dirs {
		for d := dir; d != "." && d != "/" && d != ""; d = path.Dir(d) {
			all[d] = struct{}{}
		}
	}

	ordered := make([]string, 0, len(all))
	for d := range all {
		ordered = append(ordered, d)
	}
	// A child path is its parent plus more, so reverse lexical order puts every child
	// ahead of its parent.
	sort.Sort(sort.Reverse(sort.StringSlice(ordered)))
	return ordered
}

// isEmptyDir reports whether dir holds no entries at all, including unmanaged ones.
func isEmptyDir(root *os.Root, dir string) bool {
	f, err := root.Open(dir)
	if err != nil {
		return false
	}
	defer f.Close()
	_, err = f.Readdirnames(1)
	return err == io.EOF
}

func walkFiles(fsys fs.FS, root string, fn func(relativePath string, entry fs.DirEntry) error) error {
	return fs.WalkDir(fsys, root, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}

		relativePath := filePath
		if root != "." {
			relativePath = strings.TrimPrefix(filePath, root+"/")
		}
		return fn(relativePath, entry)
	})
}

func reconcileLegacyManagedLinksBeforeCopy(sourcePath, targetPath string) error {
	sourceRoot, err := os.OpenRoot(sourcePath)
	if err != nil {
		return fmt.Errorf("could not open source config directory: %w", err)
	}
	defer sourceRoot.Close()

	targetRoot, err := os.OpenRoot(targetPath)
	if err != nil {
		return fmt.Errorf("could not open target config directory: %w", err)
	}
	defer targetRoot.Close()

	for _, relativePath := range legacyManagedLinkPaths {
		sourceInfo, err := sourceRoot.Lstat(relativePath)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("could not inspect source legacy link %q: %w", relativePath, err)
		}
		if sourceInfo.Mode()&os.ModeSymlink == 0 {
			continue
		}

		if err := targetRoot.RemoveAll(relativePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("could not reconcile legacy link %q: %w", relativePath, err)
		}
	}
	return nil
}

func verifyLegacyManagedLinksCopied(sourceRoot, targetRoot *os.Root) error {
	for _, relativePath := range legacyManagedLinkPaths {
		sourceInfo, err := sourceRoot.Lstat(relativePath)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("could not inspect source legacy link %q: %w", relativePath, err)
		}
		if sourceInfo.Mode()&os.ModeSymlink == 0 {
			continue
		}

		sourceTarget, err := sourceRoot.Readlink(relativePath)
		if err != nil {
			return fmt.Errorf("could not read source legacy link %q: %w", relativePath, err)
		}

		targetInfo, err := targetRoot.Lstat(relativePath)
		if os.IsNotExist(err) {
			return fmt.Errorf("legacy link %q was not copied", relativePath)
		}
		if err != nil {
			return fmt.Errorf("could not inspect copied legacy link %q: %w", relativePath, err)
		}
		if targetInfo.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("legacy link %q was not restored as a symlink", relativePath)
		}

		targetTarget, err := targetRoot.Readlink(relativePath)
		if err != nil {
			return fmt.Errorf("could not read copied legacy link %q: %w", relativePath, err)
		}
		if targetTarget != sourceTarget {
			return fmt.Errorf("legacy link %q target mismatch: got %q, want %q", relativePath, targetTarget, sourceTarget)
		}
	}
	return nil
}

func verifyConfigFilesCopied(sourcePath, targetPath string) error {
	sourceRoot, err := os.OpenRoot(sourcePath)
	if err != nil {
		return fmt.Errorf("could not open source config directory: %w", err)
	}
	defer sourceRoot.Close()

	targetRoot, err := os.OpenRoot(targetPath)
	if err != nil {
		return fmt.Errorf("could not open target config directory: %w", err)
	}
	defer targetRoot.Close()

	err = walkFiles(sourceRoot.FS(), ".", func(relativePath string, _ fs.DirEntry) error {
		if !strings.EqualFold(path.Ext(relativePath), ".yaml") {
			return nil
		}

		_, err := targetRoot.Lstat(relativePath)
		if os.IsNotExist(err) {
			return fmt.Errorf("config file %q was not copied", relativePath)
		}
		if err != nil {
			return fmt.Errorf("could not check copied config file %q: %w", relativePath, err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return verifyLegacyManagedLinksCopied(sourceRoot, targetRoot)
}
