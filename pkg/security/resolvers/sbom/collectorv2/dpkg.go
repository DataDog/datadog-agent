// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package collectorv2 holds sbom related files
package collectorv2

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/textproto"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	debVersion "github.com/knqyf263/go-deb-version"

	sbomtypes "github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom/types"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

type dpkgScanner struct {
}

func (s *dpkgScanner) Name() string {
	return "dpkg"
}

func (s *dpkgScanner) ListPackages(_ context.Context, root *os.Root) ([]sbomtypes.PackageWithInstalledFiles, error) {
	pkgs, err := s.listInstalledPkgs(root)
	if err != nil {
		return nil, err
	}

	installedFiles, err := s.listInstalledFiles(root)
	if err != nil {
		return nil, err
	}

	pkgsWithFiles := make([]sbomtypes.PackageWithInstalledFiles, 0, len(pkgs))
	for _, pkg := range pkgs {
		pkgsWithFiles = append(pkgsWithFiles, sbomtypes.PackageWithInstalledFiles{
			Package:        pkg,
			InstalledFiles: installedFiles[pkg.Name],
		})
	}

	return pkgsWithFiles, nil
}

const statusPath = "var/lib/dpkg/status"
const statusDPath = "var/lib/dpkg/status.d/"
const infoPath = "var/lib/dpkg/info/"
const readDirBatchSize = 32
const md5sumsSuffix = ".md5sums"

func (s *dpkgScanner) listInstalledPkgs(root *os.Root) ([]sbomtypes.Package, error) {
	pkgs, err := s.parseStatusFile(root, statusPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse dpkg status file (%s): %w", statusPath, err)
	}

	statusDDir, err := root.Open(statusDPath)
	if err != nil {
		if os.IsNotExist(err) {
			return pkgs, nil
		}
		return nil, fmt.Errorf("failed to open dpkg status.d directory (%s): %w", statusDPath, err)
	}
	defer statusDDir.Close()

	for {
		statusFiles, err := statusDDir.ReadDir(readDirBatchSize)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("failed to read dpkg status.d directory (%s): %w", statusDPath, err)
		}

		for _, statusFile := range statusFiles {
			// on distroless images, there are some md5sums files in the status.d directory
			// ignore them
			if strings.HasSuffix(statusFile.Name(), md5sumsSuffix) {
				continue
			}

			fullPath := filepath.Join(statusDPath, statusFile.Name())
			pkg, err := s.parseStatusFile(root, fullPath)
			if err != nil {
				return nil, fmt.Errorf("failed to parse dpkg status file (%s): %w", fullPath, err)
			}
			pkgs = append(pkgs, pkg...)
		}
	}

	return pkgs, nil
}

func (s *dpkgScanner) listInstalledFiles(root *os.Root) (map[string][]string, error) {
	// first with the main info dir
	installedFilesInfo, err := s.listInstalledFilesFromDir(root, infoPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	// then with the status.d dir for distroless
	installedFilesStatus, err := s.listInstalledFilesFromDir(root, statusDPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	// merge both maps, info dir has priority
	res := make(map[string][]string, len(installedFilesInfo)+len(installedFilesStatus))
	maps.Copy(res, installedFilesStatus)
	maps.Copy(res, installedFilesInfo)
	return res, nil
}

func (s *dpkgScanner) listInstalledFilesFromDir(root *os.Root, baseDir string) (map[string][]string, error) {
	infoDir, err := root.Open(baseDir)
	if err != nil {
		return nil, err
	}
	defer infoDir.Close()

	res := make(map[string][]string)

	for {
		infoFiles, err := infoDir.ReadDir(readDirBatchSize)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("failed to read dpkg info directory (%s): %w", baseDir, err)
		}

		for _, infoFile := range infoFiles {
			fileName := infoFile.Name()
			if !strings.HasSuffix(fileName, md5sumsSuffix) {
				continue
			}
			pkgName := strings.TrimSuffix(fileName, md5sumsSuffix)

			installedFiles, err := s.parseInfoFile(root, filepath.Join(baseDir, fileName))
			if err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					seclog.Warnf("failed to parse dpkg info file (%s): %v", fileName, err)
				}
				continue
			}

			res[pkgName] = installedFiles
		}
	}

	return res, nil
}

func (s *dpkgScanner) parseInfoFile(root *os.Root, path string) ([]string, error) {
	f, err := root.Open(path)
	if err != nil {

		return nil, fmt.Errorf("failed to open dpkg info file (%s): %w", path, err)
	}
	defer f.Close()

	var installedFiles []string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		// according to the doc the md5sums file are formatted as:
		// <md5sum><2 spaces><path>
		// https: //man7.org/linux/man-pages/man5/deb-md5sums.5.html
		// but some files have a single space, especially mongodb-database-tools
		// so we cut on the first space and then trim the path
		_, installedPath, ok := strings.Cut(scanner.Text(), " ")
		if !ok {
			return nil, errors.New("failed to parse installed file line, bad format")
		}
		installedPath = strings.TrimSpace(installedPath)
		installedFiles = append(installedFiles, "/"+installedPath)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan %s: %w", path, err)
	}

	return installedFiles, nil
}

var dpkgSrcCaptureRegexp = regexp.MustCompile(`([^\s]*)(?: \((.*)\))?`)

func (s *dpkgScanner) parseStatusFile(root *os.Root, path string) ([]sbomtypes.Package, error) {
	f, err := root.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var pkgs []sbomtypes.Package

	scanner := newDPKGStatusScanner(f)
	for scanner.Scan() {
		header, err := scanner.Header()
		if !errors.Is(err, io.EOF) && err != nil {
			seclog.Warnf("Parse error, filepath=%s: %v", path, err)
			continue
		}

		if !isInstalledFromStatus(header.Get("Status")) {
			continue
		}

		pkg := sbomtypes.Package{
			Name:    header.Get("Package"),
			Version: header.Get("Version"),
		}
		if pkg.Name == "" || pkg.Version == "" {
			continue
		}

		if src := header.Get("Source"); src != "" {
			matches := dpkgSrcCaptureRegexp.FindStringSubmatch(src)
			if matches != nil {
				// name would be in matches[1], but we don't use it for now
				pkg.SrcVersion = strings.TrimSpace(matches[2])
			}
		}

		if pkg.SrcVersion == "" {
			pkg.SrcVersion = pkg.Version
		}

		if v, err := debVersion.NewVersion(pkg.Version); err != nil {
			seclog.Warnf("failed to parse dpkg package version, filepath=%s, package=%s, version=%s: %v", path, pkg.Name, pkg.Version, err)
		} else {
			pkg.Version = v.Version()
			pkg.Epoch = v.Epoch()
			pkg.Release = v.Revision()
		}

		if v, err := debVersion.NewVersion(pkg.SrcVersion); err != nil {
			seclog.Warnf("failed to parse dpkg package source version, filepath=%s, package=%s, version=%s: %v", path, pkg.Name, pkg.SrcVersion, err)
		} else {
			pkg.SrcVersion = v.Version()
			pkg.SrcEpoch = v.Epoch()
			pkg.SrcRelease = v.Revision()
		}

		pkgs = append(pkgs, pkg)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan %s: %w", path, err)
	}

	return pkgs, nil
}

type dpkgStatusScanner struct {
	*bufio.Scanner
}

// newDPKGStatusScanner returns a new scanner that splits on empty lines.
func newDPKGStatusScanner(r io.Reader) *dpkgStatusScanner {
	s := bufio.NewScanner(r)
	// Package data may exceed default buffer size
	// Increase the buffer default size by 2 times
	buf := make([]byte, 0, 128*1024)
	s.Buffer(buf, 128*1024)

	s.Split(emptyLineSplit)
	return &dpkgStatusScanner{Scanner: s}
}

// Scan advances the scanner to the next token.
func (s *dpkgStatusScanner) Scan() bool {
	return s.Scanner.Scan()
}

// Header returns the MIME header of the current scan.
func (s *dpkgStatusScanner) Header() (textproto.MIMEHeader, error) {
	b := s.Bytes()
	reader := textproto.NewReader(bufio.NewReader(bytes.NewReader(b)))
	return reader.ReadMIMEHeader()
}

// emptyLineSplit is a bufio.SplitFunc that splits on empty lines.
func emptyLineSplit(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	if i := bytes.Index(data, []byte("\n\n")); i >= 0 {
		// We have a full empty line terminated block.
		return i + 2, data[0:i], nil
	}

	if atEOF {
		// Return the rest of the data if we're at EOF.
		return len(data), data, nil
	}

	return
}

func isInstalledFromStatus(status string) bool {
	for ss := range strings.FieldsSeq(status) {
		if ss == "deinstall" || ss == "purge" {
			return false
		}
	}
	return true
}
