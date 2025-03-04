// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

// Package trivy holds the scan components
package trivy

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/xerrors"

	"github.com/aquasecurity/trivy/pkg/fanal/utils"
	"github.com/aquasecurity/trivy/pkg/fanal/walker"
	xio "github.com/aquasecurity/trivy/pkg/x/io"
)

var defaultSkipDirs = []string{
	"**/.git",
	"proc",
	"sys",
	"dev",
}

// FSWalker is the filesystem walker used for SBOM generation
type FSWalker struct {
	walker *walker.FS
}

// NewFSWalker returns a new filesystem walker
func NewFSWalker() *FSWalker {
	return &FSWalker{
		walker: walker.NewFS(),
	}
}

// Walk walks the filesystem rooted at root, calling fn for each unfiltered file.
func (w *FSWalker) Walk(root string, opt walker.Option, fn walker.WalkFunc) error {
	buildPaths := func(paths []string) []string {
		buildPaths := make([]string, len(paths))
		for i, path := range paths {
			buildPaths[i] = root + path
		}
		return buildPaths
	}
	opt.SkipFiles = w.walker.BuildSkipPaths(root, buildPaths(opt.SkipFiles))
	opt.SkipDirs = w.walker.BuildSkipPaths(root, buildPaths(opt.SkipDirs))
	opt.SkipDirs = append(opt.SkipDirs, defaultSkipDirs...)
	opt.OnlyDirs = w.walker.BuildSkipPaths(root, buildPaths(opt.OnlyDirs))

	walkDirFunc := w.WalkDirFunc(root, fn, opt)
	walkDirFunc = w.onError(walkDirFunc, opt)

	// Walk the filesystem
	if err := fs.WalkDir(os.DirFS(root), ".", walkDirFunc); err != nil {
		return xerrors.Errorf("walk dir error: %w", err)
	}

	return nil
}

// WalkDirFunc is the type of the function called by [WalkDir] to visit
// each file or directory.
func (w *FSWalker) WalkDirFunc(root string, fn walker.WalkFunc, opt walker.Option) fs.WalkDirFunc {
	return func(filePath string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrPermission) || errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}

		if !strings.HasPrefix(filePath, "/") {
			if strings.HasSuffix(root, "/") {
				filePath = root + filePath
			} else {
				filePath = root + "/" + filePath
			}
		}

		relPath, err := filepath.Rel(root, filePath)
		if err != nil {
			return xerrors.Errorf("filepath rel (%s): %w", relPath, err)
		}
		relPath = filepath.ToSlash(relPath)

		// Skip unnecessary files
		switch {
		case d.IsDir():
			if utils.SkipPath(relPath, opt.SkipDirs) {
				return filepath.SkipDir
			}
			if utils.OnlyPath(relPath, opt.OnlyDirs) {
				return filepath.SkipDir
			}
			return nil
		case !opt.AllFiles && !d.Type().IsRegular():
			return nil
		case utils.SkipPath(relPath, opt.SkipFiles):
			return nil
		case utils.OnlyPath(relPath, opt.OnlyDirs):
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return xerrors.Errorf("file info error: %w", err)
		}

		if err = fn(relPath, info, fileOpener(filePath)); err != nil {
			return xerrors.Errorf("failed to analyze file: %w", err)
		}

		return nil
	}
}

func (w *FSWalker) onError(wrapped fs.WalkDirFunc, opt walker.Option) fs.WalkDirFunc {
	return func(filePath string, d fs.DirEntry, err error) error {
		err = wrapped(filePath, d, err)
		switch {
		// Unwrap fs.SkipDir error
		case errors.Is(err, fs.SkipDir):
			return fs.SkipDir
		// Ignore permission errors
		case os.IsPermission(err):
			return nil
		case err != nil:
			if opt.ErrorCallback != nil {
				err = opt.ErrorCallback(filePath, err)
				if err == nil {
					return nil
				}
			}
			// halt traversal on any other error
			return xerrors.Errorf("unknown error with %s: %w", filePath, err)
		}
		return nil
	}
}

// fileOpener returns a function opening a file.
func fileOpener(filePath string) func() (xio.ReadSeekCloserAt, error) {
	return func() (xio.ReadSeekCloserAt, error) {
		f, err := os.Open(filePath)
		if err != nil {
			return nil, err
		}

		return f, nil
	}
}
