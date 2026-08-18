// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package configbackupimpl

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	// maxDecompressedBytes bounds a gzip bomb when reading an archive.
	maxDecompressedBytes = 100 << 20 // 100 MiB
	// maxArchiveEntries bounds the entry count when reading an archive.
	maxArchiveEntries = 10000
)

// Snapshot is one configuration snapshot, as reported by ListSnapshots.
type Snapshot struct {
	Digest       string
	Timestamp    time.Time
	ConfigDir    string
	IsExperiment bool
	DeploymentID string
	AgentVersion string
	Size         int64
}

// ListSnapshots returns the configuration snapshots present in a backup
// directory, joined against the occurrence log and the manifest sidecars. An
// archive whose content digest does not match its name is treated as invalid,
// not as history.
func ListSnapshots(backupDir string) ([]Snapshot, error) {
	records, err := readStartRecords(backupDir)
	if err != nil {
		return nil, err
	}

	// Last-seen time per digest, from the occurrence log.
	lastSeen := map[string]time.Time{}
	var order []string
	for _, r := range records {
		if _, ok := lastSeen[r.Digest]; !ok {
			order = append(order, r.Digest)
		}
		lastSeen[r.Digest] = r.Timestamp
	}

	// Manifest metadata per digest.
	manifests := map[string]Manifest{}
	if read, err := ReadManifests(backupDir); err == nil {
		for _, m := range read {
			manifests[m.Digest] = m
		}
	}

	var snapshots []Snapshot
	for _, digest := range order {
		archivePath := filepath.Join(backupDir, digest+archiveSuffix)
		info, err := os.Stat(archivePath)
		if err != nil {
			continue
		}
		if !validArchiveName(digest) {
			continue
		}
		// A reader must not trust names inside the archive; validate the
		// content digest against the name before treating it as history.
		if !archiveMatchesDigest(archivePath, digest) {
			continue
		}
		s := Snapshot{
			Digest:    digest,
			Timestamp: lastSeen[digest],
			Size:      info.Size(),
		}
		if m, ok := manifests[digest]; ok {
			s.ConfigDir = m.ConfigDir
			s.IsExperiment = m.IsExperiment
			s.DeploymentID = m.DeploymentID
			s.AgentVersion = m.AgentVersion
		}
		snapshots = append(snapshots, s)
	}
	return snapshots, nil
}

// validArchiveName checks the structural shape of a digest used as an archive
// name: a 64-char lowercase hex string.
func validArchiveName(digest string) bool {
	if len(digest) != 64 {
		return false
	}
	for _, c := range digest {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// archiveMatchesDigest re-derives the content digest from the archive and
// compares it to the expected digest. It caps decompressed bytes and entry
// count to bound a gzip bomb, and rejects unsafe entry names.
func archiveMatchesDigest(archivePath, expected string) bool {
	f, err := os.Open(archivePath)
	if err != nil {
		return false
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return false
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	type entry struct {
		name    string
		content []byte
	}
	var entries []entry
	var total int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return false
		}
		if len(entries) >= maxArchiveEntries {
			return false
		}
		if !safeArchiveName(hdr.Name) {
			return false
		}
		e := entry{name: hdr.Name}
		switch hdr.Typeflag {
		case tar.TypeReg:
			if hdr.Size > maxDecompressedBytes-total {
				return false
			}
			buf := make([]byte, hdr.Size)
			if _, err := io.ReadFull(tr, buf); err != nil {
				return false
			}
			e.content = buf
			total += hdr.Size
		case tar.TypeSymlink:
			e.content = []byte("link:" + hdr.Linkname)
		default:
			return false
		}
		entries = append(entries, e)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })

	h := sha256.New()
	for _, e := range entries {
		h.Write([]byte(e.name))
		h.Write([]byte{0})
		h.Write(e.content)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)) == expected
}

// safeArchiveName rejects entry names that could escape the extraction root:
// absolute paths, paths containing a .. element after cleaning, and (on
// Windows) drive letters or backslashes.
func safeArchiveName(name string) bool {
	if name == "" {
		return false
	}
	if filepath.IsAbs(name) {
		return false
	}
	if runtime.GOOS == "windows" {
		if strings.Contains(name, "\\") {
			return false
		}
		if len(name) >= 2 && name[1] == ':' {
			return false
		}
	}
	cleaned := filepath.Clean(name)
	for _, part := range strings.Split(cleaned, string(filepath.Separator)) {
		if part == ".." {
			return false
		}
	}
	return true
}

// ReadManifests reads all manifest sidecars in the backup directory.
func ReadManifests(backupDir string) ([]Manifest, error) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return nil, err
	}
	var manifests []Manifest
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), manifestSuffix) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(backupDir, e.Name()))
		if err != nil {
			continue
		}
		var m Manifest
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		manifests = append(manifests, m)
	}
	sort.Slice(manifests, func(i, j int) bool { return manifests[i].Timestamp.Before(manifests[j].Timestamp) })
	return manifests, nil
}
