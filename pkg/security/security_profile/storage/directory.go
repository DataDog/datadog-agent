// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package storage holds files related to storages for security profiles
package storage

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2/simplelru"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
)

const directoryPerm = fs.FileMode(0750)

type profileEntry struct {
	selector  cgroupModel.WorkloadSelector
	filePaths []string // a same profile can be stored using multiple file formats
}

// Directory is a local storage for security profiles
type Directory struct {
	// Directory where the security profiles are stored
	directoryPath string
	// Maximum number of security profiles to keep
	maxProfiles int

	profilesLock sync.RWMutex
	// profiles is a LRU cache that keeps track of profiles stored by the directory. Profiles are indexed by their name.
	profiles *simplelru.LRU[string, *profileEntry]

	// stats
	deletedCount *atomic.Uint64
}

func createDir(dir string) error {
	if err := os.MkdirAll(dir, directoryPerm); err != nil && !os.IsExist(err) {
		return fmt.Errorf("couldn't create directory [%s]: %w", dir, err)
	}
	return nil
}

// fileHasProfileExtension returns true when the file extension is the .profile format.
// Only .profile files create LRU entries and drive profile retention (capped at maxProfiles):
// the SecurityProfile proto round-trips the selector directly, whereas the other formats
// (json/protobuf/dot SecDumps) only carry it indirectly. Counting other formats here would
// shrink effective retention and pull undecodable files into the load path, so they never
// create entries on their own — they are only attached to an existing profile's entry.
func fileHasProfileExtension(path string) bool {
	format, err := config.ParseStorageFormat(filepath.Ext(path))
	return err == nil && format == config.Profile
}

// fileHasStorageExtension returns true when the file extension matches any persisted storage
// format. Used to gather every on-disk file belonging to a profile so the disk-size metric
// (SizesBySelector) and the eviction callback account for all formats, not just .profile.
func fileHasStorageExtension(path string) bool {
	_, err := config.ParseStorageFormat(filepath.Ext(path))
	return err == nil
}

// profileNameFromFile returns the profile name encoded in a storage filename. Persist writes
// files as "<name>.<format>" (with an optional ".gz" suffix), so the name is the filename
// with those extensions stripped. This lets us group sibling formats of the same profile.
func profileNameFromFile(filename string) string {
	filename = strings.TrimSuffix(filename, ".gz")
	return strings.TrimSuffix(filename, filepath.Ext(filename))
}

// pickProfileFile returns the .profile file path among the given paths, or "" if none.
// Only the .profile format round-trips the SecurityProfile proto (and thus the selector),
// so it is the only format decoded back into a Profile during Load.
func pickProfileFile(paths []string) string {
	for _, path := range paths {
		if fileHasProfileExtension(path) {
			return path
		}
	}
	return ""
}

type profileFile struct {
	path  string
	mTime time.Time
}

// NewDirectory creates a new Directory instance, loading the existing profiles from the provided directory path
func NewDirectory(directoryPath string, maxProfiles int) (*Directory, error) {
	deletedCount := atomic.NewUint64(0)
	profiles, err := simplelru.NewLRU[string, *profileEntry](maxProfiles, func(deletedName string, entry *profileEntry) {
		// This callback is expected to be called with profilesLock write-lock held
		for _, filePath := range entry.filePaths {
			if err := os.Remove(filePath); err != nil {
				seclog.Errorf("failed to remove file [%s] for profile %s: %v", deletedName, filePath, err)
			}
		}

		deletedCount.Inc()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create LRU cache: %w", err)
	}

	files, err := os.ReadDir(directoryPath)
	if err != nil {
		if os.IsNotExist(err) {
			files = nil
			if err := createDir(directoryPath); err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("couldn't list files in the provided directory: %w", err)
		}
	}

	// Gather every persisted storage file grouped by profile name. All formats are tracked
	// (not just .profile) so SizesBySelector() and the eviction callback account for the full
	// on-disk footprint, but only .profile files create LRU entries below.
	filePathsByName := make(map[string][]string)
	var profileFiles []*profileFile
	for _, file := range files {
		if !fileHasStorageExtension(file.Name()) {
			continue
		}

		fileInfo, err := file.Info()
		if err != nil {
			seclog.Warnf("failed to retrieve file [%s] information: %s", file.Name(), err)
			continue
		}

		if !fileInfo.Mode().IsRegular() {
			continue
		}

		fullPath := filepath.Join(directoryPath, file.Name())
		filePathsByName[profileNameFromFile(file.Name())] = append(filePathsByName[profileNameFromFile(file.Name())], fullPath)

		if fileHasProfileExtension(file.Name()) {
			profileFiles = append(profileFiles, &profileFile{
				path:  fullPath,
				mTime: fileInfo.ModTime(),
			})
		}
	}

	// sort from oldest to newest so the most recent profiles survive the LRU cap
	slices.SortFunc(profileFiles, func(a, b *profileFile) int {
		if a.mTime.Equal(b.mTime) {
			return 0
		} else if a.mTime.Before(b.mTime) {
			return -1
		}
		return 1
	})

	for _, file := range profileFiles {
		pProto, err := profile.LoadProtoFromFile(file.path)
		if err != nil {
			seclog.Warnf("failed to load profile from file [%s]: %s", file.path, err)
			continue
		}
		if pProto.Metadata == nil {
			seclog.Warnf("profile loaded from file [%s] has no metadata", file.path)
			continue
		}
		if pProto.Selector == nil {
			seclog.Warnf("profile loaded from file [%s] has no selector", file.path)
			continue
		}

		// Attach every persisted format of this profile (json/protobuf/dot siblings) so the
		// disk-size metric counts them and eviction removes them all. Fall back to just the
		// .profile file if the naming is unexpected and no siblings were grouped.
		paths := filePathsByName[pProto.Metadata.Name]
		if len(paths) == 0 {
			paths = []string{file.path}
		}

		profiles.Add(pProto.Metadata.Name, &profileEntry{
			selector:  cgroupModel.ProtoToWorkloadSelector(pProto.Selector),
			filePaths: paths,
		})
	}

	return &Directory{
		directoryPath: directoryPath,
		maxProfiles:   maxProfiles,
		profiles:      profiles,
		deletedCount:  deletedCount,
	}, nil
}

// Persist persists the provided profile to the directory
func (d *Directory) Persist(request config.StorageRequest, p *profile.Profile, raw *bytes.Buffer) error {
	filename := fmt.Sprintf("%s.%s", p.Metadata.Name, request.Format.String())

	if request.Compression {
		compressedData, err := compressWithGZip(filename, raw.Bytes())
		if err != nil {
			return err
		}
		raw = compressedData
		filename += ".gz"
	}

	filePath := filepath.Join(d.directoryPath, filename)

	tmpFilePath := filePath + ".tmp"

	if err := createDir(d.directoryPath); err != nil {
		return err
	}

	file, err := os.Create(tmpFilePath)
	if err != nil {
		return fmt.Errorf("couldn't persist to file [%s]: %w", tmpFilePath, err)
	}
	defer file.Close()

	// set output file access mode
	if err := file.Chmod(0400); err != nil {
		return fmt.Errorf("couldn't set mod for file [%s]: %w", file.Name(), err)
	}

	// persist data to disk
	if _, err := file.Write(raw.Bytes()); err != nil {
		return fmt.Errorf("couldn't write to file [%s]: %w", file.Name(), err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("could not close file [%s]: %w", file.Name(), err)
	}

	if err := os.Rename(tmpFilePath, filePath); err != nil {
		return fmt.Errorf("couldn't rename file from [%s] to [%s]: %w", tmpFilePath, filePath, err)
	}

	seclog.Infof("[%s] file for [%s] written at: [%s]", request.Format, p.GetSelectorStr(), filePath)

	d.profilesLock.Lock()
	entry, ok := d.profiles.Get(p.Metadata.Name)
	if ok && !slices.Contains(entry.filePaths, filePath) { // the file can already exist if the profile was updated
		entry.filePaths = append(entry.filePaths, filePath)
	} else if !ok {
		d.profiles.Add(p.Metadata.Name, &profileEntry{
			selector:  *p.GetWorkloadSelector(),
			filePaths: []string{filePath},
		})
	}
	d.profilesLock.Unlock()

	return nil
}

// Load loads the profile for the provided selector if it exists
func (d *Directory) Load(wls *cgroupModel.WorkloadSelector, p *profile.Profile) (bool, error) {
	if wls == nil {
		return false, errors.New("no selector was provided")
	}

	if p == nil {
		return false, errors.New("no profile was provided")
	}

	d.profilesLock.RLock()
	defer d.profilesLock.RUnlock()

	for _, entry := range d.profiles.Values() {
		if !entry.selector.Match(*wls) {
			continue
		}

		// Prefer the .profile format because it round-trips the selector explicitly via the
		// SecurityProfile proto. Other formats (json/protobuf/dot) encode SecDump, which only
		// preserves the selector indirectly through tags.
		chosen := pickProfileFile(entry.filePaths)
		if chosen == "" {
			continue
		}
		if err := p.Decode(chosen); err != nil {
			return false, fmt.Errorf("failed to decode profile [%s]: %s", chosen, err)
		}
		return true, nil
	}

	return false, nil
}

// GetStorageType returns the storage type
func (d *Directory) GetStorageType() config.StorageType {
	return config.LocalStorage
}

// SizesBySelector returns the on-disk size in bytes of each stored profile, keyed by workload selector.
// Multiple file formats for the same selector are summed together.
func (d *Directory) SizesBySelector() map[cgroupModel.WorkloadSelector]int64 {
	d.profilesLock.RLock()
	defer d.profilesLock.RUnlock()

	result := make(map[cgroupModel.WorkloadSelector]int64, d.profiles.Len())
	for _, entry := range d.profiles.Values() {
		var size int64
		for _, filePath := range entry.filePaths {
			if info, err := os.Stat(filePath); err == nil {
				size += info.Size()
			}
		}
		result[entry.selector] += size
	}
	return result
}

// totalSizeOnDisk scans the directory and sums the size of every file with a known storage format
// extension. Used for the aggregate disk-usage metric because the LRU only tracks the primary
// .profile files even when other formats (e.g. .json) are configured and persisted alongside.
func (d *Directory) totalSizeOnDisk() int64 {
	files, err := os.ReadDir(d.directoryPath)
	if err != nil {
		seclog.Warnf("couldn't list files in %s, %s metric may be inaccurate: %v", d.directoryPath, metrics.MetricActivityDumpLocalStorageSizeOnDisk, err)
		return 0
	}

	var total int64
	for _, file := range files {
		fullpath := filepath.Join(d.directoryPath, file.Name())
		if _, err := config.ParseStorageFormat(filepath.Ext(fullpath)); err != nil {
			continue
		}
		info, err := os.Stat(fullpath)
		if err != nil {
			continue
		}
		total += info.Size()
	}
	return total
}

// SendTelemetry sends telemetry for the current storage
func (d *Directory) SendTelemetry(sender statsd.ClientInterface) {
	d.profilesLock.RLock()
	if count := d.profiles.Len(); count > 0 {
		_ = sender.Gauge(metrics.MetricActivityDumpLocalStorageCount, float64(count), nil, 1.0)
	}
	d.profilesLock.RUnlock()

	if count := d.deletedCount.Swap(0); count > 0 {
		_ = sender.Count(metrics.MetricActivityDumpLocalStorageDeleted, int64(count), nil, 1.0)
	}

	_ = sender.Gauge(metrics.MetricActivityDumpLocalStorageSizeOnDisk, float64(d.totalSizeOnDisk()), nil, 1.0)
}

func compressWithGZip(filename string, rawBuf []byte) (*bytes.Buffer, error) {
	var buf bytes.Buffer

	zw := gzip.NewWriter(&buf)
	zw.Name = strings.TrimSuffix(filename, ".gz")
	zw.ModTime = time.Now()

	if _, err := zw.Write(rawBuf); err != nil {
		return nil, fmt.Errorf("couldn't compress activity dump: %w", err)
	}
	// Closing the gzip stream also flushes it
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("couldn't compress activity dump: %w", err)
	}

	return &buf, nil
}
