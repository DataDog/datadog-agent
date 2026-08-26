// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build trivy

package trivy

import (
	"bufio"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/aquasecurity/trivy/pkg/types"
	"github.com/twmb/murmur3"

	"github.com/DataDog/datadog-agent/pkg/sbom/telemetry"
	"github.com/DataDog/datadog-agent/pkg/sbom/usage"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// binaryLangTypes are the result types whose target is a single executable
// holding every package the result reports. An exec of that one file means each
// of them really is in use, so they are all anchored to its path.
var binaryLangTypes = []ftypes.LangType{
	ftypes.GoBinary,
	ftypes.RustBinary,
	ftypes.PythonExecutable,
	ftypes.NodeJsExecutable,
	ftypes.PhpExecutable,
	ftypes.JavaExecutable,
}

// usrMergedDirs are the top-level directories a merged-/usr layout symlinks into
// /usr, where a package database and a kernel-resolved path disagree on the
// prefix of the same file.
var usrMergedDirs = []string{"/bin/", "/sbin/", "/lib/", "/lib64/"}

// indexEntry is one (file, component) pair before the table is sorted by hash.
// A file can name several components, so entries are not unique by path.
type indexEntry struct {
	path string
	ref  uint32
}

// BuildUsageIndex derives the file-to-component table of report, so a runtime
// observer can attribute a file access to a component occurrence of the final
// SBOM built from the same scan. componentRefs maps Trivy's in-process package
// UID to the final payload BOM ref; indexID is that payload's serial number.
//
// Files come from three places, all of them already in the report:
//
//   - InstalledFiles, which Trivy fills for rpm, dpkg and apk;
//   - a package's own FilePath, which the artifact analyzers set to the jar,
//     gemspec or dist-info they read;
//   - the result target, for a result whose target is one executable holding
//     every package it reports, a Go binary or a language runtime.
//
// A package that could not be mapped uniquely to a final BOM ref is kept, with
// its files, for system-probe package resolution but cannot be stamped. Lock-file
// results are skipped: their target says the package manager ran, not that any
// dependency was used, and artifact analyzers cover installed packages.
//
// root is the filesystem the report describes, or "" when the scan read an image
// rather than a directory. It is used only to widen a Python wheel from the
// dist-info Trivy names to the code files an import actually opens.
func BuildUsageIndex(report *types.Report, componentRefs map[string]string, indexID string, scan usage.ScanID, generation uint64, root string) *usage.Index {
	idx := &usage.Index{Scan: scan, Generation: generation, IndexID: indexID, Status: usage.Ready}

	var entries []indexEntry
	add := func(file string, ref uint32) {
		if file == "" {
			return
		}
		entries = append(entries, indexEntry{path: absClean(file), ref: ref})
	}

	for _, result := range report.Results {
		binaryTarget := ""
		if result.Class == types.ClassLangPkg && slices.Contains(binaryLangTypes, result.Type) {
			binaryTarget = result.Target
		}

		for _, pkg := range result.Packages {
			ref := uint32(len(idx.Components))
			bomRef := componentRefs[pkg.Identifier.UID]
			if bomRef == "" {
				idx.UnmappedComponents++
			}
			idx.Components = append(idx.Components, usage.Component{
				BOMRef:      bomRef,
				Purl:        purlOf(pkg),
				Name:        pkg.Name,
				Version:     pkg.Version,
				Epoch:       pkg.Epoch,
				Release:     pkg.Release,
				SrcVersion:  pkg.SrcVersion,
				SrcEpoch:    pkg.SrcEpoch,
				SrcRelease:  pkg.SrcRelease,
				Application: pkg.Relationship == ftypes.RelationshipRoot,
			})

			for _, file := range pkg.InstalledFiles {
				add(file, ref)
			}
			add(pkg.FilePath, ref)
			add(binaryTarget, ref)

			if result.Type == ftypes.PythonPkg {
				for _, file := range pythonCodeFiles(root, pkg.FilePath) {
					add(file, ref)
				}
			}
		}
	}

	entries = append(entries, usrMergedAliases(entries)...)
	fill(idx, entries)
	return idx
}

// usrMergedAliases returns the alternate /usr spelling of every entry whose
// alternate no other entry already claims.
//
// On a merged-/usr layout the package database records one spelling of a file
// while the kernel resolves the other, so both have to be in the table. The
// report itself says which layout this is: a split layout has /bin/sh and
// /usr/bin/sh as separate entries owned by whichever packages own them, so the
// alternate is already claimed and no alias is added. Deciding it here costs one
// pass and needs no access to the filesystem the report describes.
func usrMergedAliases(entries []indexEntry) []indexEntry {
	claimed := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		claimed[e.path] = struct{}{}
	}

	var aliases []indexEntry
	for _, e := range entries {
		alias := ""
		if after, ok := strings.CutPrefix(e.path, "/usr"); ok && hasUsrMergedDir(after) {
			alias = after
		} else if hasUsrMergedDir(e.path) {
			alias = "/usr" + e.path
		}
		if alias == "" {
			continue
		}
		if _, taken := claimed[alias]; taken {
			continue
		}
		aliases = append(aliases, indexEntry{path: alias, ref: e.ref})
	}
	return aliases
}

// fill sorts entries by path hash into the index, dropping any hash two distinct
// paths share. A hash carrying several components is legitimate, one file holding
// many packages, but a collision would attribute a file to a package that does
// not own it, and silently missing beats silently wrong.
func fill(idx *usage.Index, entries []indexEntry) {
	fillWithHasher(idx, entries, murmur3.StringSum64)
}

// fillWithHasher exists so collision handling can be tested without depending
// on an accidental collision in a fixed hash implementation.
func fillWithHasher(idx *usage.Index, entries []indexEntry, hashPath func(string) uint64) {
	type hashed struct {
		hash uint64
		path string
		ref  uint32
	}

	all := make([]hashed, 0, len(entries))
	for _, e := range entries {
		all = append(all, hashed{hash: hashPath(e.path), path: e.path, ref: e.ref})
	}
	slices.SortFunc(all, func(a, b hashed) int {
		if a.hash != b.hash {
			if a.hash < b.hash {
				return -1
			}
			return 1
		}
		return int(a.ref) - int(b.ref)
	})

	idx.Hashes = make([]uint64, 0, len(all))
	idx.Refs = make([]uint32, 0, len(all))
	idx.Paths = make([]string, 0, len(all))

	collisions := 0
	for i := 0; i < len(all); {
		j := i
		distinct := true
		for ; j < len(all) && all[j].hash == all[i].hash; j++ {
			if all[j].path != all[i].path {
				distinct = false
			}
		}
		if distinct {
			for k := i; k < j; k++ {
				// One path repeated for the same component adds nothing.
				if k > i && all[k].ref == all[k-1].ref {
					continue
				}
				idx.Hashes = append(idx.Hashes, all[k].hash)
				idx.Refs = append(idx.Refs, all[k].ref)
				idx.Paths = append(idx.Paths, all[k].path)
			}
		} else {
			collisions++
		}
		i = j
	}

	if collisions > 0 {
		idx.HashCollisions = collisions
		for range collisions {
			telemetry.SBOMUsagePathHashCollisions.Inc()
		}
		log.Warnf("SBOM usage index for %s dropped %d colliding path hashes", idx.Scan, collisions)
	}
}

// pythonCodeFiles returns the installed files of the wheel whose dist-info
// metadata Trivy read, taken from the RECORD beside it.
//
// Trivy anchors a wheel to its METADATA file, which CPython never opens: an
// import reads the .py and .pyc files instead, so the anchor alone reports every
// imported wheel idle. RECORD lists them, and reading it needs the filesystem
// the report describes. A scan of an image rather than a directory has none, so
// there the wheel keeps the METADATA anchor.
func pythonCodeFiles(root, metadataPath string) []string {
	if root == "" || metadataPath == "" {
		return nil
	}
	distInfo := path.Dir(metadataPath)
	if !strings.HasSuffix(distInfo, ".dist-info") {
		return nil
	}

	// root names a directory of the host running the scan while distInfo is a
	// path inside the filesystem being scanned, so the two are joined with the
	// separator of the host and the scanned paths stay slash-separated.
	f, err := os.Open(filepath.Join(root, filepath.FromSlash(distInfo), "RECORD"))
	if err != nil {
		return nil
	}
	defer f.Close()

	// RECORD is CSV of path,hash,size relative to the directory holding
	// dist-info. Only the path matters here, and a path with an embedded comma is
	// quoted, which no importable module name produces.
	var files []string
	sitePackages := path.Dir(distInfo)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		name, _, _ := strings.Cut(scanner.Text(), ",")
		if name == "" || strings.HasPrefix(name, "\"") {
			continue
		}
		files = append(files, path.Join(sitePackages, name))
	}
	if err := scanner.Err(); err != nil {
		log.Debugf("could not read %s/RECORD: %v", distInfo, err)
	}
	return files
}

// purlOf returns the package URL Trivy assigned pkg, or "" when it has none.
func purlOf(pkg ftypes.Package) string {
	if pkg.Identifier.PURL == nil {
		return ""
	}
	return pkg.Identifier.PURL.String()
}

func absClean(p string) string {
	return path.Clean("/" + strings.TrimPrefix(p, "/"))
}

func hasUsrMergedDir(p string) bool {
	for _, dir := range usrMergedDirs {
		if strings.HasPrefix(p, dir) {
			return true
		}
	}
	return false
}
