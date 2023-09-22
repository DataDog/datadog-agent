// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package dump holds dump related files
package dump

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2/simplelru"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

type dumpFiles struct {
	Name  string
	Files []string
	MTime time.Time
}

type dumpFilesSlice []*dumpFiles

func newDumpFilesSlice(dumps map[string]*dumpFiles) dumpFilesSlice {
	s := make(dumpFilesSlice, 0, len(dumps))
	for _, ad := range dumps {
		s = append(s, ad)
	}
	return s
}

// Len is part of sort.Interface
func (s dumpFilesSlice) Len() int {
	return len(s)
}

// Swap is part of sort.Interface
func (s dumpFilesSlice) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Less is part of sort.Interface. The MTime timestamp is used to compare two entries.
func (s dumpFilesSlice) Less(i, j int) bool {
	return s[i].MTime.Before(s[j].MTime)
}

// ActivityDumpLocalStorage is used to manage ActivityDumps storage
type ActivityDumpLocalStorage struct {
	sync.Mutex
	deletedCount *atomic.Uint64
	localDumps   *simplelru.LRU[string, *[]string]
}

// NewActivityDumpLocalStorage creates a new ActivityDumpLocalStorage instance
func NewActivityDumpLocalStorage(cfg *config.Config, m *ActivityDumpManager) (ActivityDumpStorage, error) {
	adls := &ActivityDumpLocalStorage{
		deletedCount: atomic.NewUint64(0),
	}

	var err error
	adls.localDumps, err = simplelru.NewLRU(cfg.RuntimeSecurity.ActivityDumpLocalStorageMaxDumpsCount, func(name string, files *[]string) {
		if len(*files) == 0 {
			return
		}

		// notify the security profile directory provider that we're about to delete a profile
		if m.securityProfileManager != nil {
			m.securityProfileManager.OnLocalStorageCleanup(*files)
		}

		// remove everything
		for _, f := range *files {
			_ = os.Remove(f)
		}

		adls.deletedCount.Add(1)
	})
	if err != nil {
		return nil, fmt.Errorf("couldn't create the dump LRU: %w", err)
	}

	// snapshot the dumps in the default output directory
	if len(cfg.RuntimeSecurity.ActivityDumpLocalStorageDirectory) > 0 {
		// list all the files in the activity dump output directory
		files, err := os.ReadDir(cfg.RuntimeSecurity.ActivityDumpLocalStorageDirectory)
		if err != nil {
			if os.IsNotExist(err) {
				files = make([]os.DirEntry, 0)
				if err = os.MkdirAll(cfg.RuntimeSecurity.ActivityDumpLocalStorageDirectory, 0750); err != nil {
					return nil, fmt.Errorf("couldn't create output directory for cgroup activity dumps: %w", err)
				}
			} else {
				return nil, fmt.Errorf("couldn't list existing activity dumps in the provided cgroup output directory: %w", err)
			}
		}

		// merge the files to insert them in the LRU
		localDumps := make(map[string]*dumpFiles)
		for _, f := range files {
			// check if the extension of the file is known
			ext := filepath.Ext(f.Name())
			if _, err = config.ParseStorageFormat(ext); err != nil && ext != ".gz" {
				// ignore this file
				continue
			}
			// retrieve the basename of the dump
			dumpName := strings.TrimSuffix(filepath.Base(f.Name()), ext)
			// insert the file in the list of dumps
			ad, ok := localDumps[dumpName]
			if !ok {
				ad = &dumpFiles{
					Name:  dumpName,
					Files: make([]string, 1),
				}
				localDumps[dumpName] = ad
			}
			ad.Files = append(ad.Files, f.Name())
			dumpInfo, err := f.Info()
			if err != nil {
				// ignore this file
				continue
			}
			if !ad.MTime.IsZero() && ad.MTime.Before(dumpInfo.ModTime()) {
				ad.MTime = dumpInfo.ModTime()
			}
		}
		// sort the existing dumps by modification timestamp
		dumps := newDumpFilesSlice(localDumps)
		sort.Sort(dumps)
		// insert the dumps in cache (will trigger clean up if necessary)
		for _, ad := range dumps {
			newFiles := ad.Files
			adls.localDumps.Add(ad.Name, &newFiles)
		}
	}

	return adls, nil
}

// GetStorageType returns the storage type of the ActivityDumpLocalStorage
func (storage *ActivityDumpLocalStorage) GetStorageType() config.StorageType {
	return config.LocalStorage
}

// Persist saves the provided buffer to the persistent storage
func (storage *ActivityDumpLocalStorage) Persist(request config.StorageRequest, ad *ActivityDump, raw *bytes.Buffer) error {
	storage.Lock()
	defer storage.Unlock()

	outputPath := request.GetOutputPath(ad.Metadata.Name)

	if request.Compression {
		tmpRaw, err := compressWithGZip(path.Base(outputPath), raw.Bytes())
		if err != nil {
			return err
		}
		raw = tmpRaw
	}

	// set activity dump size for current encoding
	ad.Metadata.Size = uint64(len(raw.Bytes()))

	// add the file to the list of local dumps (thus removing one or more files if we reached the limit)
	if storage.localDumps != nil {
		files, ok := storage.localDumps.Get(ad.Metadata.Name)
		if !ok {
			storage.localDumps.Add(ad.Metadata.Name, &[]string{outputPath})
		} else {
			*files = append(*files, outputPath)
		}
	}

	// create output file
	_ = os.MkdirAll(request.OutputDirectory, 0400)
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("couldn't persist to file [%s]: %w", outputPath, err)
	}
	defer file.Close()

	// set output file access mode
	if err = os.Chmod(outputPath, 0400); err != nil {
		return fmt.Errorf("couldn't set mod for file [%s]: %w", outputPath, err)
	}

	// persist data to disk
	if _, err = file.Write(raw.Bytes()); err != nil {
		return fmt.Errorf("couldn't write to file [%s]: %w", outputPath, err)
	}

	if err = file.Close(); err != nil {
		return fmt.Errorf("could not close file [%s]: %w", file.Name(), err)
	}

	seclog.Infof("[%s] file for [%s] written at: [%s]", request.Format, ad.GetSelectorStr(), outputPath)
	return nil
}

// SendTelemetry sends telemetry for the current storage
func (storage *ActivityDumpLocalStorage) SendTelemetry(sender sender.Sender) {
	storage.Lock()
	defer storage.Unlock()

	// send the count of dumps stored locally
	if count := storage.localDumps.Len(); count > 0 {
		sender.Gauge(metrics.MetricActivityDumpLocalStorageCount, float64(count), "", []string{})
	}

	// send the count of recently deleted dumps
	if count := storage.deletedCount.Swap(0); count > 0 {
		sender.Count(metrics.MetricActivityDumpLocalStorageDeleted, float64(count), "", []string{})
	}
}
