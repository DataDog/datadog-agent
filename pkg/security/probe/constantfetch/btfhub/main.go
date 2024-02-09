// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && linux_bpf

// Package main holds main related files
package main

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/smira/go-xz"
	"golang.org/x/sync/semaphore"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/probe/constantfetch"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	utilKernel "github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func main() {
	var archiveRootPath string
	var constantOutputPath string
	var forceRefresh bool

	flag.StringVar(&archiveRootPath, "archive-root", "", "Root path of BTFHub archive")
	flag.StringVar(&constantOutputPath, "output", "", "Output path for JSON constants")
	flag.BoolVar(&forceRefresh, "force-refresh", false, "Force refresh of the constants")
	flag.Parse()

	archiveCommit, err := getCommitSha(archiveRootPath)
	if err != nil {
		fmt.Printf("error fetching btfhub-archive commit: %v\n", err)
	}
	fmt.Printf("btfhub-archive: commit %s\n", archiveCommit)

	preAllocHint := 0

	if !forceRefresh {
		// skip if commit is already the most recent
		currentConstants, err := getCurrentConstants(constantOutputPath)
		if err == nil && currentConstants.Commit != "" {
			if currentConstants.Commit == archiveCommit {
				fmt.Printf("already at most recent archive commit")
				return
			}
			preAllocHint = len(currentConstants.Kernels)
		}
	}

	twCollector := newTreeWalkCollector(preAllocHint)

	if err := filepath.WalkDir(archiveRootPath, twCollector.treeWalkerBuilder(archiveRootPath)); err != nil {
		panic(err)
	}

	export := twCollector.finish()

	export.Commit = archiveCommit

	fmt.Printf("%d kernels\n", len(export.Kernels))
	fmt.Printf("%d unique constants\n", len(export.Constants))

	output, err := json.MarshalIndent(export, "", "\t")
	if err != nil {
		panic(err)
	}

	if err := os.WriteFile(constantOutputPath, output, 0644); err != nil {
		panic(err)
	}
}

func getCurrentConstants(path string) (*constantfetch.BTFHubConstants, error) {
	cjson, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var currentConstants constantfetch.BTFHubConstants
	if err := json.Unmarshal(cjson, &currentConstants); err != nil {
		return nil, err
	}

	return &currentConstants, nil
}

func getCommitSha(cwd string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = cwd

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

type treeWalkCollector struct {
	counter int
	// wg is used so that the finish waits on all kernels
	wg sync.WaitGroup
	// sem is used to limit the amount of parallel extractions
	sem       *semaphore.Weighted
	resultsMu sync.Mutex
	results   []extractionResult
}

func newTreeWalkCollector(preAllocHint int) *treeWalkCollector {
	return &treeWalkCollector{
		counter: 0,
		sem:     semaphore.NewWeighted(int64(runtime.NumCPU() * 2)),
		results: make([]extractionResult, 0, preAllocHint),
	}
}

func (c *treeWalkCollector) finish() constantfetch.BTFHubConstants {
	c.wg.Wait()

	sort.Slice(c.results, func(i, j int) bool { return c.results[i].index < c.results[j].index })

	constants := make([]map[string]uint64, 0)
	kernels := make([]constantfetch.BTFHubKernel, 0)

	for _, res := range c.results {
		index := -1
		for i, other := range constants {
			if reflect.DeepEqual(other, res.constants) {
				index = i
				break
			}
		}

		if index == -1 {
			index = len(constants)
			constants = append(constants, res.constants)
		}

		kernels = append(kernels, constantfetch.BTFHubKernel{
			Distribution:   res.distribution,
			DistribVersion: res.distribVersion,
			Arch:           res.arch,
			UnameRelease:   res.unameRelease,
			ConstantsIndex: index,
		})
	}

	return constantfetch.BTFHubConstants{
		Constants: constants,
		Kernels:   kernels,
	}
}

func (c *treeWalkCollector) treeWalkerBuilder(prefix string) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".tar.xz") {
			return nil
		}

		btfRunIndex := c.counter
		c.counter++

		pathSuffix := strings.TrimPrefix(path, prefix)

		btfParts := strings.Split(pathSuffix, "/")
		if len(btfParts) < 4 {
			return fmt.Errorf("file has wrong format: %s", pathSuffix)
		}

		distribution := btfParts[len(btfParts)-4]
		distribVersion := btfParts[len(btfParts)-3]
		arch := btfParts[len(btfParts)-2]
		unameRelease := strings.TrimSuffix(btfParts[len(btfParts)-1], ".btf.tar.xz")

		c.wg.Add(1)
		if err := c.sem.Acquire(context.TODO(), 1); err != nil {
			return fmt.Errorf("failed to acquire sem token: %w", err)
		}

		go func() {
			defer func() {
				c.wg.Done()
				c.sem.Release(1)
			}()

			fmt.Println(btfRunIndex, path)

			constants, err := extractConstantsFromBTF(path, distribution, distribVersion)
			if err != nil {
				seclog.Errorf("failed to extract constants from `%s`: %v", path, err)
				return
			}

			res := extractionResult{
				index:          btfRunIndex,
				distribution:   distribution,
				distribVersion: distribVersion,
				arch:           arch,
				unameRelease:   unameRelease,
				constants:      constants,
			}

			c.resultsMu.Lock()
			c.results = append(c.results, res)
			c.resultsMu.Unlock()
		}()

		return nil
	}
}

type extractionResult struct {
	index          int
	distribution   string
	distribVersion string
	arch           string
	unameRelease   string
	constants      map[string]uint64
}

func extractConstantsFromBTF(archivePath, distribution, distribVersion string) (map[string]uint64, error) {
	btfReader, err := createBTFReaderFromTarball(archivePath)
	if err != nil {
		return nil, err
	}

	archiveFileName := path.Base(archivePath)
	btfFileName := strings.TrimSuffix(archiveFileName, ".tar.xz")

	releasePart := strings.Split(btfFileName, "-")[0]
	kvCode, err := utilKernel.ParseReleaseString(releasePart)
	if err != nil {
		return nil, err
	}

	osRelease := map[string]string{
		"ID":         distribution,
		"VERSION_ID": distribVersion,
	}
	kv := &kernel.Version{
		Code:      kvCode,
		OsRelease: osRelease,
	}

	fetcher, err := constantfetch.NewBTFConstantFetcherFromReader(btfReader)
	if err != nil {
		return nil, err
	}

	probe.AppendProbeRequestsToFetcher(fetcher, kv)
	return fetcher.FinishAndGetResults()
}

func createBTFReaderFromTarball(archivePath string) (io.ReaderAt, error) {
	archiveFile, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer archiveFile.Close()

	xzReader, err := xz.NewReader(archiveFile)
	if err != nil {
		return nil, err
	}
	defer xzReader.Close()

	tarReader := tar.NewReader(xzReader)

	btfBuffer := bytes.NewBuffer([]byte{})
outer:
	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read entry from tarball: %w", err)
		}

		switch hdr.Typeflag {
		case tar.TypeReg:
			if strings.HasSuffix(hdr.Name, ".btf") {
				if _, err := io.Copy(btfBuffer, tarReader); err != nil {
					return nil, fmt.Errorf("failed to uncompress file %s: %w", hdr.Name, err)
				}
				break outer
			}
		}
	}

	return bytes.NewReader(btfBuffer.Bytes()), nil
}
