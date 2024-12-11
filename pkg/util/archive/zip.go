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

	securejoin "github.com/cyphar/filepath-securejoin"
)

// fileInfo is an adapted implementation of FileInfo from
// https://github.com/mholt/archiver/blob/v3.5.1/archiver.go#L125
// Copyright (c) 2016 Matthew Holt
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

// Zip is an adapted implementation of (*Zip).Archive from
// https://github.com/mholt/archiver/blob/v3.5.1/zip.go#L140
// Copyright (c) 2016 Matthew Holt
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
			return fmt.Errorf("error making directory %s: %w", dir, err)
		}
	}

	outputFile, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("error creating %s: %w", destination, err)
	}
	defer outputFile.Close()

	zipWriter := zip.NewWriter(outputFile)
	zipWriter.RegisterCompressor(zip.Deflate, func(outputFile io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(outputFile, flate.DefaultCompression)
	})
	defer zipWriter.Close()

	for _, source := range sources {
		err := writeWalk(zipWriter, source, destination)
		if err != nil {
			return fmt.Errorf("error walking %s: %w", source, err)
		}
	}

	return nil
}

// Unzip unpacks the .zip file at source to destination.
func Unzip(source, destination string) error {
	destinationDir := filepath.Dir(destination)

	if !fileExists(destinationDir) {
		err := os.MkdirAll(destinationDir, 0755)
		if err != nil {
			return fmt.Errorf("preparing destination: %v", err)
		}
	}

	zipReader, err := zip.OpenReader(source)
	if err != nil {
		return fmt.Errorf("opening source file: %v", err)
	}
	defer zipReader.Close()

	for _, f := range zipReader.File {
		err := extractAndWriteFile(f, destination)
		if err != nil {
			return err
		}
	}

	return nil
}

func extractAndWriteFile(f *zip.File, targetRootFolder string) error {
	if f.Mode()&os.ModeSymlink != 0 {
		// We skip symlink for security reasons
		return nil
	}

	archiveFile, err := f.Open()
	if err != nil {
		return fmt.Errorf("failed to open file: %v", err)
	}
	defer archiveFile.Close()

	targetFilepath, err := securejoin.SecureJoin(targetRootFolder, f.Name)

	if err != nil {
		return fmt.Errorf("illegal file path: %s", targetFilepath)
	}

	if f.FileInfo().IsDir() {
		err := os.MkdirAll(targetFilepath, 0755)
		if err != nil {
			return fmt.Errorf("failed to create dir %s: %v", targetFilepath, err)
		}
	} else {
		err := os.MkdirAll(filepath.Dir(targetFilepath), 0755)
		if err != nil {
			return fmt.Errorf("failed to file dir %s: %v", targetFilepath, err)
		}
		targetFileDescriptor, err := os.OpenFile(targetFilepath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %v", targetFilepath, err)
		}
		_, err = io.Copy(targetFileDescriptor, archiveFile)
		defer targetFileDescriptor.Close()

		if err != nil {
			return fmt.Errorf("failed to copy file %s: %v", targetFilepath, err)
		}
	}

	return nil
}

// fileExists is an adapted implementation of fileExists from
// https://github.com/mholt/archiver/blob/v3.5.1/archiver.go#L279
// Copyright (c) 2016 Matthew Holt
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, fs.ErrNotExist)
}

// writeWalk is an adapted implementation of (*Zip).writeWalk from
// https://github.com/mholt/archiver/blob/v3.5.1/zip.go#L300
// Copyright (c) 2016 Matthew Holt
func writeWalk(zipWriter *zip.Writer, source, destination string) error {
	sourceInfo, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("error stat:  %s: %w", source, err)
	}
	destAbs, err := filepath.Abs(destination)
	if err != nil {
		return fmt.Errorf("error getting absolute path of destination %s %s: %w", destination, source, err)
	}

	return filepath.Walk(source, func(fpath string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("error traversing  %s: %w", fpath, err)
		}
		if info == nil {
			return fmt.Errorf("%s: no file info", fpath)
		}

		if info.Mode()&os.ModeSymlink != 0 {
			// We skip symlink for security reasons
			return nil
		}

		// make sure we do not copy the output file into the output
		// file; that results in an infinite loop and disk exhaustion!
		fpathAbs, err := filepath.Abs(fpath)
		if err != nil {
			return fmt.Errorf("error getting absolute path %s: %w", fpath, err)
		}

		if within(fpathAbs, destAbs) {
			return nil
		}

		// build the name to be used within the archive
		nameInArchive, err := makeNameInArchive(sourceInfo, source, "", fpath)
		if err != nil {
			return err
		}

		finfo := fileInfo{
			FileInfo:   info,
			customName: nameInArchive,
		}
		header, err := zip.FileInfoHeader(finfo)
		if err != nil {
			return fmt.Errorf("error getting header %s: %w", finfo.Name(), err)
		}

		if finfo.IsDir() {
			header.Name += "/"
			header.Method = zip.Store
		} else {
			header.Method = zip.Deflate
		}

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return fmt.Errorf("error making header %s: %w", finfo.Name(), err)
		}

		if finfo.IsDir() {
			// Nothing to write for directories
			return nil
		}

		if info.Mode().IsRegular() {
			file, err := os.Open(fpath)
			if err != nil {
				return fmt.Errorf("error opening %s: %w", fpath, err)
			}
			defer file.Close()

			_, err = io.Copy(writer, file)
			if err != nil {
				return fmt.Errorf("error copying contents %s: %w", finfo.Name(), err)
			}
		}

		return nil
	})
}

// makeNameInArchive is an adapted implementation of makeNameInArchive from
// https://github.com/mholt/archiver/blob/v3.5.1/archiver.go#L413
// Copyright (c) 2016 Matthew Holt
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
