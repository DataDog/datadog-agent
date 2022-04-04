// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

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
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/log"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/probe/constantfetch"
	utilKernel "github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/smira/go-xz"
	"golang.org/x/sync/semaphore"
)

func main() {
	var archiveRootPath string
	var constantOutputPath string

	flag.StringVar(&archiveRootPath, "archive-root", "", "Root path of BTFHub archive")
	flag.StringVar(&constantOutputPath, "output", "", "Output path for JSON constants")
	flag.Parse()

	twCollector := newTreeWalkCollector()

	if err := filepath.WalkDir(archiveRootPath, twCollector.treeWalkerBuilder(archiveRootPath)); err != nil {
		panic(err)
	}

	export := twCollector.finish()
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

type treeWalkCollector struct {
	counter   int
	wg        sync.WaitGroup
	sem       *semaphore.Weighted
	resultsMu sync.Mutex
	results   []extractionResult
}

func newTreeWalkCollector() *treeWalkCollector {
	return &treeWalkCollector{
		counter: 0,
		sem:     semaphore.NewWeighted(int64(runtime.NumCPU() * 2)),
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
				log.Errorf("failed to extract constants from `%s`: %v", path, err)
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

	xzReader, err := xz.NewReader(archiveFile)
	if err != nil {
		return nil, err
	}

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
