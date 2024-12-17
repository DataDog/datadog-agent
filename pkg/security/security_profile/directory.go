// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package securityprofile holds security profiles related files
package securityprofile

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
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
	filePaths []string
	selectors []cgroupModel.WorkloadSelector
}

// Directory is a local storage for security profiles
type Directory struct {
	// Directory where the security profiles are stored
	directoryPath string
	// Maximum number of security profiles to keep
	maxProfiles int

	mappingsLock sync.RWMutex
	// selectorToName allows finding a security profile from a given selector
	// selector to names is a 1-to-N mapping (because multiple profiles can be created for the same selector)
	selectorToNames map[cgroupModel.WorkloadSelector][]string
	// namesToFiles allows finding the files associated from a given security profile name
	// name to files is a 1-to-N mapping (because a same profile can be stored with multiple file formats)
	nameToEntry *simplelru.LRU[string, *profileEntry]

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

func updateMappings(selectorToNames map[cgroupModel.WorkloadSelector][]string, nameToEntry *simplelru.LRU[string, *profileEntry], selector *cgroupModel.WorkloadSelector, name string, path string, versions []string) {
	versionSelector := *selector
	for _, version := range versions {
		versionSelector.Tag = version
		selectorToNames[versionSelector] = append(selectorToNames[versionSelector], name)
	}

	entry, ok := nameToEntry.Get(name)
	if !ok {
		newEntry := &profileEntry{
			filePaths: []string{path},
		}
		for _, version := range versions {
			versionSelector.Tag = version
			newEntry.selectors = append(newEntry.selectors, versionSelector)
		}
		// nameToEntry.Add might call the eviction callback with mappingsLock held
		nameToEntry.Add(name, newEntry)
	} else {
		entry.filePaths = append(entry.filePaths, path)
		for _, version := range versions {
			versionSelector.Tag = version
			entry.selectors = append(entry.selectors, versionSelector)
		}
	}
}

type profileFile struct {
	path  string
	mTime time.Time
}

// NewDirectory creates a new Directory instance, loading the existing profiles from the provided directory path
func NewDirectory(directoryPath string, maxProfiles int) (*Directory, error) {
	selectorToNames := make(map[cgroupModel.WorkloadSelector][]string)
	deletedCount := atomic.NewUint64(0)
	nameToEntry, err := simplelru.NewLRU[string, *profileEntry](maxProfiles, func(deletedName string, entry *profileEntry) {
		// This callback is expected to be called with mappingsLock write-lock held
		for _, filePath := range entry.filePaths {
			if err := os.Remove(filePath); err != nil {
				seclog.Errorf("failed to remove file [%s]: %s", filePath, err)
			}
		}

		for _, selector := range entry.selectors {
			selectorToNames[selector] = slices.DeleteFunc(selectorToNames[selector], func(name string) bool {
				return name == deletedName
			})
			if len(selectorToNames[selector]) == 0 {
				delete(selectorToNames, selector)
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

	profileFiles := make(map[string]*profileFile)
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

		path := filepath.Join(directoryPath, file.Name())
		_, ok := profileFiles[path]
		if !ok {
			profileFiles[path] = &profileFile{
				path:  path,
				mTime: fileInfo.ModTime(),
			}
		}
	}

	fileSlice := make([]*profileFile, 0, len(profileFiles))
	for _, file := range profileFiles {
		fileSlice = append(fileSlice, file)
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
		selector := cgroupModel.ProtoToWorkloadSelector(pProto.Selector)
		versions := make([]string, 0, len(pProto.ProfileContexts))
		for key := range pProto.ProfileContexts {
			versions = append(versions, key)
		}

		updateMappings(selectorToNames, nameToEntry, &selector, pProto.Metadata.GetName(), file.path, versions)
	}

	return &Directory{
		directoryPath:   directoryPath,
		maxProfiles:     maxProfiles,
		selectorToNames: selectorToNames,
		nameToEntry:     nameToEntry,
		deletedCount:    deletedCount,
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

	if err := createDir(d.directoryPath); err != nil {
		return err
	}

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("couldn't persist to file [%s]: %w", filePath, err)
	}
	defer file.Close()

	// set output file access mode
	if err := os.Chmod(filePath, 0400); err != nil {
		return fmt.Errorf("couldn't set mod for file [%s]: %w", filePath, err)
	}

	// persist data to disk
	if _, err := file.Write(raw.Bytes()); err != nil {
		return fmt.Errorf("couldn't write to file [%s]: %w", filePath, err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("could not close file [%s]: %w", file.Name(), err)
	}

	seclog.Infof("[%s] file for [%s] written at: [%s]", request.Format, p.GetSelectorStr(), filePath)

	d.mappingsLock.Lock()
	updateMappings(d.selectorToNames, d.nameToEntry, p.GetWorkloadSelector(), p.Metadata.Name, filePath, p.GetVersions())
	d.mappingsLock.Unlock()

	return nil
}

// Load loads the profile for the provided selector if it exists
func (d *Directory) Load(wls *cgroupModel.WorkloadSelector, p *profile.Profile) (bool, error) {
	if wls == nil {
		return false, fmt.Errorf("no selector was provided")
	}

	if p == nil {
		return false, fmt.Errorf("no profile was provided")
	}

	d.mappingsLock.RLock()
	defer d.mappingsLock.RUnlock()

	for selector, names := range d.selectorToNames {
		if selector.Match(*wls) {
			for _, name := range names {
				entry, ok := d.nameToEntry.Get(name)
				if !ok {
					continue
				}
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
	}

	return false, nil
}

// GetStorageType returns the storage type
func (d *Directory) GetStorageType() config.StorageType {
	return config.LocalStorage
}

// SendTelemetry sends telemetry for the current storage
func (d *Directory) SendTelemetry(sender statsd.ClientInterface) {
	d.mappingsLock.RLock()
	// send the count of dumps stored locally
	if count := d.nameToEntry.Len(); count > 0 {
		_ = sender.Gauge(metrics.MetricActivityDumpLocalStorageCount, float64(count), nil, 1.0)
	}
	d.mappingsLock.RUnlock()

	// send the count of recently deleted dumps
	if count := d.deletedCount.Swap(0); count > 0 {
		_ = sender.Count(metrics.MetricActivityDumpLocalStorageDeleted, int64(count), nil, 1.0)
	}
}
