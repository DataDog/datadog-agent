// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package archive provides functions to archive and unarchive files.
package archive

import (
	"archive/zip"
	"compress/flate"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Zip is an adapted implementation of (*Zip).Archive from
// https://github.com/mholt/archiver/blob/v3.5.1/zip.go#L140
// under MIT License.
func Zip(sources []string, destination string) error {
	if !strings.HasSuffix(destination, ".zip") {
		return fmt.Errorf("%s must have a .zip extension", destination)
	}
	if fileExists(destination) {
		return fmt.Errorf("file already exists: %s", destination)
	}
	dir := filepath.Dir(destination)
	if !fileExists(dir) {
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			return fmt.Errorf("%w making directory: %s", err, dir)
		}
	}

	out, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("%w creating %s", err, destination)
	}
	defer out.Close()

	zipW := zip.NewWriter(out)
	zipW.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(out, flate.DefaultCompression)
	})
	defer zipW.Close()

	for _, source := range sources {
		err := writeWalk(zipW, source, destination)
		if err != nil {
			return fmt.Errorf("%w walking %s", err, source)
		}
	}

	return nil
}

// UnZip unpacks the .zip file at source to destination.
func UnZip(source, destination string) error {
	destinationDir := filepath.Dir(destination)

	if !fileExists(destinationDir) {
		err := os.MkdirAll(destinationDir, 0755)
		if err != nil {
			return fmt.Errorf("preparing destination: %v", err)
		}
	}

	zipR, err := zip.OpenReader(source)
	if err != nil {
		return fmt.Errorf("opening source file: %v", err)
	}
	defer zipR.Close()

	// Closure to address file descriptors issue with all the deferred .Close() methods
	extractAndWriteFile := func(f *zip.File) error {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		path := filepath.Join(destination, f.Name)

		// Check for ZipSlip (Directory traversal)
		if !strings.HasPrefix(path, filepath.Clean(destination)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", path)
		}

		if f.FileInfo().IsDir() {
			err := os.MkdirAll(path, f.Mode())
			if err != nil {
				return err
			}
		} else {
			err := os.MkdirAll(filepath.Dir(path), f.Mode())
			if err != nil {
				return err
			}
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = io.Copy(f, rc)
			if err != nil {
				return err
			}
		}
		return nil
	}

	for _, f := range zipR.File {
		err := extractAndWriteFile(f)
		if err != nil {
			return err
		}
	}

	return nil
}

// fileExists is an adapted implementation of fileExists from
// https://github.com/mholt/archiver/blob/v3.5.1/archiver.go#L279
// under MIT License.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, fs.ErrNotExist)
}

// fileInfo is an adapted implementation of FileInfo from
// https://github.com/mholt/archiver/blob/v3.5.1/archiver.go#L125
// under MIT license.
type fileInfo struct {
	os.FileInfo
	customName string
}

func (f fileInfo) Name() string {
	if f.customName != "" {
		return f.customName
	}
	return f.FileInfo.Name()
}

// writeWalk is an adapted implementation of (*Zip).writeWalk from
// https://github.com/mholt/archiver/blob/v3.5.1/zip.go#L300
// under MIT License.
func writeWalk(zipW *zip.Writer, source, destination string) error {
	sourceInfo, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("%w stat: %s", err, source)
	}
	destAbs, err := filepath.Abs(destination)
	if err != nil {
		return fmt.Errorf("%w getting absolute path of destination %s: %s", err, destination, source)
	}

	return filepath.Walk(source, func(fpath string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("%w traversing %s", err, fpath)
		}
		if info == nil {
			return fmt.Errorf("%s: no file info", fpath)
		}

		fpathAbs, err := filepath.Abs(fpath)
		if err != nil {
			return fmt.Errorf("%w %s: getting absolute path", err, fpath)
		}
		if within(fpathAbs, destAbs) {
			return nil
		}

		// build the name to be used within the archive
		nameInArchive, err := makeNameInArchive(sourceInfo, source, "", fpath)
		if err != nil {
			return err
		}

		var file io.ReadCloser
		if info.Mode().IsRegular() {
			file, err = os.Open(fpath)
			if err != nil {
				return fmt.Errorf("%w %s: opening", err, fpath)
			}
			defer file.Close()
		}

		finfo := fileInfo{
			FileInfo:   info,
			customName: nameInArchive,
		}
		header, err := zip.FileInfoHeader(finfo)
		if err != nil {
			return fmt.Errorf("%w %s: getting header", err, finfo.Name())
		}

		if finfo.IsDir() {
			header.Name += "/"
		}
		header.Method = zip.Store

		writer, err := zipW.CreateHeader(header)
		if err != nil {
			return fmt.Errorf("%w %s: making header", err, finfo.Name())
		}

		if finfo.IsDir() {
			return nil
		}

		_, err = io.Copy(writer, file)
		if err != nil {
			return fmt.Errorf("%w %s: copying contents", err, finfo.Name())
		}

		return nil
	})
}

// makeNameInArchive is an adapted implementation of makeNameInArchive from
// https://github.com/mholt/archiver/blob/v3.5.1/archiver.go#L413
// under MIT License.
//
// makeNameInArchive returns the filename for the file given by fpath to be used within
// the archive. sourceInfo is the FileInfo obtained by calling os.Stat on source, and baseDir
// is an optional base directory that becomes the root of the archive. fpath should be the
// unaltered file path of the file given to a filepath.WalkFunc.
func makeNameInArchive(sourceInfo os.FileInfo, source, baseDir, fpath string) (string, error) {
	name := filepath.Base(fpath) // start with the file or dir name
	if sourceInfo.IsDir() {
		// preserve internal directory structure; that's the path components
		// between the source directory's leaf and this file's leaf
		dir, err := filepath.Rel(filepath.Dir(source), filepath.Dir(fpath))
		if err != nil {
			return "", err
		}
		// prepend the internal directory structure to the leaf name,
		// and convert path separators to forward slashes as per spec
		name = path.Join(filepath.ToSlash(dir), name)
	}
	return path.Join(baseDir, name), nil // prepend the base directory
}

// This function is inspired by https://github.com/mholt/archiver/blob/v3.5.1/archiver.go#L360
// within returns true if sub is within or equal to parent.
func within(parent, sub string) bool {
	rel, err := filepath.Rel(parent, sub)
	if err != nil {
		return false
	}
	return !strings.Contains(rel, "..")
}
