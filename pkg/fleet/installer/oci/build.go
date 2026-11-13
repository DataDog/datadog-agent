// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oci

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/DataDog/datadog-agent/pkg/zstd"
)

// time0 zeroes tar entry timestamps so packages are reproducible.
var time0 = time.Time{}

// zstdLayer is a v1.Layer whose blob is zstd-compressed with the Datadog
// package layer media type; its diff ID is the hash of the uncompressed tar.
type zstdLayer struct {
	blob      []byte
	diffID    v1.Hash
	digest    v1.Hash
	computed  sync.Once
	mediaType types.MediaType
}

func newZstdLayer(tarBytes []byte, mt types.MediaType) (v1.Layer, error) {
	compressed, err := zstdCompress(tarBytes)
	if err != nil {
		return nil, err
	}
	diffID, _, err := v1.SHA256(bytes.NewReader(tarBytes))
	if err != nil {
		return nil, err
	}
	return &zstdLayer{blob: compressed, diffID: diffID, mediaType: mt}, nil
}

func (l *zstdLayer) Digest() (v1.Hash, error) {
	var err error
	l.computed.Do(func() {
		l.digest, _, err = v1.SHA256(bytes.NewReader(l.blob))
	})
	return l.digest, err
}

func (l *zstdLayer) DiffID() (v1.Hash, error) { return l.diffID, nil }

func (l *zstdLayer) Size() (int64, error) { return int64(len(l.blob)), nil }

func (l *zstdLayer) MediaType() (types.MediaType, error) { return l.mediaType, nil }

func (l *zstdLayer) Compressed() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(l.blob)), nil
}

func (l *zstdLayer) Uncompressed() (io.ReadCloser, error) {
	reader, err := zstd.NewReader(bytes.NewReader(l.blob))
	if err != nil {
		return nil, err
	}
	out := &bytes.Buffer{}
	if _, err := io.Copy(out, reader); err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(out.Bytes())), nil
}

var _ v1.Layer = (*zstdLayer)(nil)

// depsGroup is the integrations-core optional dependency group that pulls in
// a check's third-party dependencies (Requires-Dist: name==version; extra == "deps").
const depsGroup = "deps"

// distInfoPattern matches a wheel dist-info directory name, e.g.
// datadog_mysql-7.5.1.dist-info or PyMySQL-1.1.0.dist-info.
var distInfoPattern = regexp.MustCompile(`^(.*)-([0-9][^-]*(?:\.[0-9]+)*(?:\.[a-zA-Z0-9]+)*)\.dist-info$`)

// requiresDistPattern matches a METADATA Requires-Dist entry, e.g.
// "Requires-Dist: psutil==6.0.0; extra == \"deps\"" or
// "Requires-Dist: cryptography>=42.0.0".
var requiresDistPattern = regexp.MustCompile(`^Requires-Dist:\s*([A-Za-z0-9._-]+)\s*([^;]*)\s*(?:;\s*(.*))?$`)

// DistInfo describes one installed distribution in a site-packages tree.
type DistInfo struct {
	// RawName is the distribution name as installed (datadog_mysql, PyMySQL).
	RawName string
	// Name is the normalized name (datadog_mysql, pymysql).
	Name string
	// Dir is the absolute path of the dist-info directory.
	Dir string
	// Requires holds the parsed Requires-Dist entries.
	Requires []Requirement
	// TopLevelPaths are the site-packages-relative top-level paths installed
	// by this distribution, from its RECORD file.
	TopLevelPaths []string
}

// Requirement is a parsed Requires-Dist entry.
type Requirement struct {
	// Name is the normalized distribution name.
	Name string
	// Kind classifies the requirement marker.
	Kind requirementKind
}

type requirementKind int

const (
	// requireHard is a requirement without any environment marker.
	requireHard requirementKind = iota
	// requireDepsGroup is a requirement guarded by the deps optional group
	// marker.
	requireDepsGroup
	// requireOtherExtra is a requirement guarded by any other optional-group
	// marker (test, dev, docs, ...): never part of the runtime install.
	requireOtherExtra
)

// SitePackages describes a python site-packages tree.
type SitePackages struct {
	// Dir is the absolute path of the site-packages directory.
	Dir string
	// PythonDir is the python lib directory name, e.g. python3.12.
	PythonDir string
	// Dists maps normalized distribution names to their information.
	Dists map[string]*DistInfo
}

// NormalizeDistName normalizes a distribution name to the python import-name
// form: lowercase with '-' replaced by '_'.
func NormalizeDistName(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "-", "_"))
}

// ParseSitePackages scans a site-packages directory and parses the METADATA
// and RECORD files of every installed distribution.
func ParseSitePackages(dir string) (*SitePackages, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return nil, fmt.Errorf("could not read %s: %w", absDir, err)
	}
	site := &SitePackages{
		Dir:       absDir,
		PythonDir: filepath.Base(filepath.Dir(absDir)),
		Dists:     map[string]*DistInfo{},
	}
	for _, entry := range entries {
		match := distInfoPattern.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		dist := &DistInfo{RawName: match[1], Name: NormalizeDistName(match[1]), Dir: filepath.Join(absDir, entry.Name())}
		if err := parseMetadata(dist); err != nil {
			return nil, err
		}
		if err := parseRecord(dist); err != nil {
			return nil, err
		}
		site.Dists[dist.Name] = dist
	}
	return site, nil
}

func parseMetadata(dist *DistInfo) error {
	data, err := os.ReadFile(filepath.Join(dist.Dir, "METADATA"))
	if err != nil {
		return fmt.Errorf("could not read METADATA of %s: %w", dist.RawName, err)
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if !strings.HasPrefix(line, "Requires-Dist:") {
			continue
		}
		match := requiresDistPattern.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if match == nil {
			continue
		}
		marker := ""
		if match[3] != "" {
			marker = match[3]
		}
		kind := requireOtherExtra
		if marker == "" {
			kind = requireHard
		} else if strings.Contains(marker, depsGroup) {
			kind = requireDepsGroup
		}
		dist.Requires = append(dist.Requires, Requirement{
			Name: NormalizeDistName(match[1]),
			Kind: kind,
		})
	}
	return nil
}

func parseRecord(dist *DistInfo) error {
	data, err := os.ReadFile(filepath.Join(dist.Dir, "RECORD"))
	if err != nil {
		return fmt.Errorf("could not read RECORD of %s: %w", dist.RawName, err)
	}
	seen := map[string]struct{}{}
	for line := range strings.SplitSeq(string(data), "\n") {
		if line == "" {
			continue
		}
		path := strings.Split(line, ",")[0]
		path = filepath.ToSlash(path)
		if path == "" || strings.HasPrefix(path, "..") || path == "." {
			continue
		}
		top := path
		if idx := strings.Index(path, "/"); idx >= 0 {
			top = path[:idx]
		}
		if _, dup := seen[top]; !dup {
			seen[top] = struct{}{}
			dist.TopLevelPaths = append(dist.TopLevelPaths, top)
		}
	}
	sort.Strings(dist.TopLevelPaths)
	return nil
}

func (s *SitePackages) checkDists() []*DistInfo {
	checks := []*DistInfo{}
	for _, dist := range s.Dists {
		if strings.HasPrefix(dist.Name, "datadog_") && dist.Name != "datadog_checks_base" {
			checks = append(checks, dist)
		}
	}
	sort.Slice(checks, func(i, j int) bool { return checks[i].Name < checks[j].Name })
	return checks
}

func (s *SitePackages) dist(name string) *DistInfo {
	return s.Dists[name]
}

// walkClosure adds to the set the transitive closure of name following the
// runtime requirements of installed distributions: hard requirements
// always, deps-group requirements only when followAll is set. Other
// optional groups (test, dev, ...) are never followed.
func (s *SitePackages) walkClosure(set map[string]struct{}, name string, followAll bool) {
	if _, done := set[name]; done {
		return
	}
	dist := s.dist(name)
	if dist == nil {
		return
	}
	set[name] = struct{}{}
	for _, req := range dist.Requires {
		if req.Kind == requireHard || (followAll && req.Kind == requireDepsGroup) {
			s.walkClosure(set, req.Name, followAll)
		}
	}
}

// DepsManifest returns the check-name to dependency-packages mapping for
// the on-demand dependency manifest: every installed check with a
// deps-group requirement depends on the given dependency package (the fat
// package today; per-check packages later only change this mapping).
func (s *SitePackages) DepsManifest(packageName string) map[string][]string {
	manifest := map[string][]string{}
	for _, check := range s.checkDists() {
		for _, req := range check.Requires {
			if req.Kind == requireDepsGroup {
				checkName := strings.TrimPrefix(check.Name, "datadog_")
				manifest[checkName] = []string{packageName}
				break
			}
		}
	}
	return manifest
}

// DepsPackageDists returns the distributions that must go into the on-demand
// dependency package: the transitive closure of every check's deps-group
// requirements, minus the distributions already shipped by the agent (the
// base check closure and the checks themselves).
func (s *SitePackages) DepsPackageDists() []*DistInfo {
	base := map[string]struct{}{}
	s.walkClosure(base, "datadog_checks_base", false)
	for _, check := range s.checkDists() {
		base[check.Name] = struct{}{}
	}

	deps := map[string]struct{}{}
	for _, check := range s.checkDists() {
		for _, req := range check.Requires {
			if req.Kind == requireDepsGroup {
				s.walkClosure(deps, req.Name, true)
			}
		}
	}

	result := []*DistInfo{}
	for name := range deps {
		if _, isBase := base[name]; isBase {
			continue
		}
		if dist := s.dist(name); dist != nil {
			result = append(result, dist)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

// BuildDepsPackageLayer returns a single layer containing the given
// distributions laid out under a top-level site-packages directory: the
// agent puts <package>/stable/site-packages on the interpreter import
// path, so the installed content is importable without any wiring.
func (s *SitePackages) BuildDepsPackageLayer(dists []*DistInfo) (tarBytes []byte, err error) {
	topLevel := map[string]struct{}{}
	for _, dist := range dists {
		for _, path := range dist.TopLevelPaths {
			topLevel[path] = struct{}{}
		}
	}
	paths := make([]string, 0, len(topLevel))
	for path := range topLevel {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	prefix := "site-packages"
	buf := &bytes.Buffer{}
	tw := tar.NewWriter(buf)
	defer func() {
		if cerr := tw.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	for _, path := range paths {
		if err := writeTarTree(tw, filepath.Join(s.Dir, path), filepath.Join(prefix, path)); err != nil {
			return nil, err
		}
	}
	if err := tw.Flush(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeTarTree(tw *tar.Writer, source, name string) error {
	return filepath.WalkDir(source, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." && !d.IsDir() {
			// the source is a single top-level file (e.g. six.py), include it
			rel = ""
		} else if rel == "." {
			return nil
		}
		headerName := filepath.ToSlash(filepath.Join(name, rel))
		info, err := d.Info()
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = headerName
		header.ModTime = time0
		header.Format = tar.FormatPAX
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			header.Typeflag = tar.TypeSymlink
			header.Linkname = filepath.ToSlash(link)
			header.Size = 0
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if header.Typeflag == tar.TypeReg {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			if _, err := io.Copy(tw, f); err != nil {
				return err
			}
		}
		return nil
	})
}

// BuildDepsPackage builds the OCI image of the on-demand dependency package
// with the manifest annotations the fleet installer downloader expects.
func (s *SitePackages) BuildDepsPackage(name, version string, dists []*DistInfo) (v1.Image, error) {
	tarBytes, err := s.BuildDepsPackageLayer(dists)
	if err != nil {
		return nil, err
	}
	layer, err := newZstdLayer(tarBytes, DatadogPackageLayerMediaType)
	if err != nil {
		return nil, err
	}
	img, err := mutate.AppendLayers(empty.Image, layer)
	if err != nil {
		return nil, fmt.Errorf("could not append dependency layer: %w", err)
	}
	size, err := layer.Size()
	if err != nil {
		return nil, err
	}
	img = mutate.Annotations(img, map[string]string{
		AnnotationPackage: name,
		AnnotationVersion: version,
		AnnotationSize:    strconv.FormatInt(size, 10),
	}).(v1.Image)
	return img, nil
}

func zstdCompress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer, err := zstd.NewWriter(&buf)
	if err != nil {
		return nil, err
	}
	if _, err := writer.Write(data); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
