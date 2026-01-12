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

func fileHasProfileExtension(path string) bool {
	format, err := config.ParseStorageFormat(filepath.Ext(path))
	return err == nil && format == config.Profile
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

	var fileSlice []*profileFile
	for _, file := range files {
		if !fileHasProfileExtension(file.Name()) {
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

		fileSlice = append(fileSlice, &profileFile{
			path:  filepath.Join(directoryPath, file.Name()),
			mTime: fileInfo.ModTime(),
		})
	}

	// sort from oldest to newest
	slices.SortFunc(fileSlice, func(a, b *profileFile) int {
		if a.mTime.Equal(b.mTime) {
			return 0
		} else if a.mTime.Before(b.mTime) {
			return -1
		}
		return 1
	})

	for _, file := range fileSlice {
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

		profiles.Add(pProto.Metadata.Name, &profileEntry{
			selector:  cgroupModel.ProtoToWorkloadSelector(pProto.Selector),
			filePaths: []string{file.path},
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
		if entry.selector.Match(*wls) {
			for _, file := range entry.filePaths {
				if !fileHasProfileExtension(file) {
					continue
				}

				if err := p.Decode(file); err != nil {
					return false, fmt.Errorf("failed to decode profile [%s]: %s", file, err)
				}
				return true, nil
			}
		}
	}

	return false, nil
}

// GetStorageType returns the storage type
func (d *Directory) GetStorageType() config.StorageType {
	return config.LocalStorage
}

// SendTelemetry sends telemetry for the current storage
func (d *Directory) SendTelemetry(sender statsd.ClientInterface) {
	d.profilesLock.RLock()
	// send the count of dumps stored locally
	if count := d.profiles.Len(); count > 0 {
		_ = sender.Gauge(metrics.MetricActivityDumpLocalStorageCount, float64(count), nil, 1.0)
	}
	d.profilesLock.RUnlock()

	// send the count of recently deleted dumps
	if count := d.deletedCount.Swap(0); count > 0 {
		_ = sender.Count(metrics.MetricActivityDumpLocalStorageDeleted, int64(count), nil, 1.0)
	}
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
