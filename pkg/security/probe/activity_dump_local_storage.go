// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/golang-lru/simplelru"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	seclog "github.com/DataDog/datadog-agent/pkg/security/log"
	"github.com/DataDog/datadog-agent/pkg/security/probe/dump"
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
	localDumps *simplelru.LRU
}

// NewActivityDumpLocalStorage creates a new ActivityDumpLocalStorage instance
func NewActivityDumpLocalStorage(p *Probe) (ActivityDumpStorage, error) {
	if p == nil {
		return &ActivityDumpLocalStorage{}, nil
	}

	lru, err := simplelru.NewLRU(p.config.ActivityDumpLocalStorageMaxDumpsCount, func(key interface{}, value interface{}) {
		files, ok := key.([]string)
		if !ok || len(files) == 0 {
			return
		}
		// remove everything
		for _, f := range files {
			_ = os.Remove(f)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("couldn't create the dump LRU: %w", err)
	}

	// snapshot the dumps in the default output directory
	if len(p.config.ActivityDumpLocalStorageDirectory) > 0 {
		// list all the files in the activity dump output directory
		files, err := os.ReadDir(p.config.ActivityDumpLocalStorageDirectory)
		if err != nil {
			if os.IsNotExist(err) {
				files = make([]os.DirEntry, 0)
				if err = os.MkdirAll(p.config.ActivityDumpLocalStorageDirectory, 0400); err != nil {
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
			if _, err = dump.ParseStorageFormat(ext); err != nil && ext != ".gz" {
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
			lru.Add(ad.Name, ad.Files)
		}
	}

	return &ActivityDumpLocalStorage{
		localDumps: lru,
	}, nil
}

// GetStorageType returns the storage type of the ActivityDumpLocalStorage
func (storage *ActivityDumpLocalStorage) GetStorageType() dump.StorageType {
	return dump.LocalStorage
}

// Persist saves the provided buffer to the persistent storage
func (storage *ActivityDumpLocalStorage) Persist(request dump.StorageRequest, ad *ActivityDump, raw *bytes.Buffer) error {
	outputPath := request.GetOutputPath(ad.DumpMetadata.Name)

	if request.Compression {
		var tmpBuf bytes.Buffer
		zw := gzip.NewWriter(&tmpBuf)
		zw.Name = strings.TrimSuffix(path.Base(outputPath), ".gz")
		zw.ModTime = time.Now()
		if _, err := zw.Write(raw.Bytes()); err != nil {
			return fmt.Errorf("couldn't compress activity dump: %w", err)
		}
		if err := zw.Flush(); err != nil {
			return fmt.Errorf("couldn't compress activity dump: %w", err)
		}
		if err := zw.Close(); err != nil {
			return fmt.Errorf("couldn't compress activity dump: %w", err)
		}
		raw = &tmpBuf
	}

	// set activity dump size for current encoding
	ad.DumpMetadata.Size = uint64(len(raw.Bytes()))

	// add the file to the list of local dumps (thus removing one or more files if we reached the limit)
	if storage.localDumps != nil {
		filesRaw, ok := storage.localDumps.Get(ad.DumpMetadata.Name)
		if !ok {
			storage.localDumps.Add(ad.DumpMetadata.Name, []string{outputPath})
		} else {
			files, ok := filesRaw.([]string)
			if !ok {
				files = []string{}
			}
			storage.localDumps.Add(ad.DumpMetadata.Name, append(files, outputPath))
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
	seclog.Infof("[%s] file for [%s] written at: [%s]", request.Format, ad.GetSelectorStr(), outputPath)
	return nil
}

// SendTelemetry sends telemetry for the current storage
func (storage *ActivityDumpLocalStorage) SendTelemetry(sender aggregator.Sender) {}
