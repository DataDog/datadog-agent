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
	"slices"
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
const diversionsPath = "var/lib/dpkg/diversions"
const readDirBatchSize = 32
const md5sumsSuffix = ".md5sums"
const listSuffix = ".list"

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
				seclog.Warnf("failed to parse dpkg status file (%s): %v", fullPath, err)
				continue
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

	diversions := s.readDiversions(root)
	for pkg, files := range res {
		for i, file := range files {
			if d, ok := diversions[file]; ok && d.pkg != pkg {
				files[i] = d.to
			}
		}
	}
	return res, nil
}

// diversion holds the path where dpkg installs the files that packages other
// than pkg ship at a diverted path. pkg is ":" for a local diversion.
type diversion struct {
	to, pkg string
}

// readDiversions returns the dpkg diversions by path. The database holds three
// lines for each: the path, its new name and the diverting package.
func (s *dpkgScanner) readDiversions(root *os.Root) map[string]diversion {
	f, err := root.Open(diversionsPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			seclog.Warnf("failed to open dpkg diversions (%s): %v", diversionsPath, err)
		}
		return nil
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		seclog.Warnf("failed to scan %s: %v", diversionsPath, err)
	}

	diversions := make(map[string]diversion, len(lines)/3)
	for i := 0; i+2 < len(lines); i += 3 {
		diversions[lines[i]] = diversion{to: lines[i+1], pkg: lines[i+2]}
	}
	return diversions
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
			infoName := strings.TrimSuffix(fileName, md5sumsSuffix)
			pkgName := infoName
			// dpkg info files for multiarch packages are named "pkg:arch.md5sums"
			// but the Package: field in the status file is unqualified ("pkg"), so strip the arch suffix.
			if i := strings.LastIndex(pkgName, ":"); i >= 0 {
				pkgName = pkgName[:i]
			}

			installedFiles, err := s.parseInfoFile(root, filepath.Join(baseDir, fileName))
			if err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					seclog.Warnf("failed to parse dpkg info file (%s): %v", fileName, err)
				}
				continue
			}

			// A package keeps the md5sums of the files another package took
			// over, and dpkg drops them from its list.
			listed, err := s.parseListFile(root, filepath.Join(baseDir, infoName+listSuffix))
			if err == nil {
				installedFiles = slices.DeleteFunc(installedFiles, func(file string) bool {
					_, ok := listed[file]
					return !ok
				})
			} else if !errors.Is(err, os.ErrNotExist) {
				seclog.Warnf("failed to parse dpkg list file (%s): %v", infoName+listSuffix, err)
			}

			res[pkgName] = append(res[pkgName], installedFiles...)
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
		_, installedPath, _ := strings.Cut(scanner.Text(), " ")
		installedPath = strings.TrimSpace(installedPath)
		if installedPath == "" {
			continue
		}
		// nfpm writes ./usr/bin/x where dpkg writes usr/bin/x
		installedFiles = append(installedFiles, filepath.Clean("/"+installedPath))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan %s: %w", path, err)
	}

	return installedFiles, nil
}

// parseListFile returns the set of paths in a dpkg .list file.
func (s *dpkgScanner) parseListFile(root *os.Root, path string) (map[string]struct{}, error) {
	f, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	listed := make(map[string]struct{})
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		listed[scanner.Text()] = struct{}{}
	}
	return listed, scanner.Err()
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
		seclog.Warnf("failed to scan %s past %d packages: %v", path, len(pkgs), err)
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
