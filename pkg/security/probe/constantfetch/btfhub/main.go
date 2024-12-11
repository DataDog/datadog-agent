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
	"cmp"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"runtime/pprof"
	"slices"
	"strings"
	"sync"

	"github.com/smira/go-xz"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/probe/constantfetch"
	utilKernel "github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func main() {
	var archiveRootPath string
	var constantOutputPath string
	var forceRefresh bool
	var combineConstants bool
	var cpuPprofPath string

	flag.StringVar(&archiveRootPath, "archive-root", "", "Root path of BTFHub archive")
	flag.StringVar(&constantOutputPath, "output", "", "Output path for JSON constants")
	flag.BoolVar(&forceRefresh, "force-refresh", false, "Force refresh of the constants")
	flag.BoolVar(&combineConstants, "combine", false, "Don't read btf files, but read constants")
	flag.StringVar(&cpuPprofPath, "cpu-prof", "", "Path to the CPU profile to generate")
	flag.Parse()

	if cpuPprofPath != "" {
		f, err := os.Create(cpuPprofPath)
		if err != nil {
			panic(err)
		}
		defer f.Close()

		if err := pprof.StartCPUProfile(f); err != nil {
			panic(err)
		}
		defer pprof.StopCPUProfile()
	}

	if combineConstants {
		combined, err := combineConstantFiles(archiveRootPath)
		if err != nil {
			panic(err)
		}
		if err := outputConstants(&combined, constantOutputPath); err != nil {
			panic(err)
		}
		return
	}

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

	var wg sync.WaitGroup
	// github actions runner have only 2 cores
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := twCollector.extractor(); err != nil {
				panic(err)
			}
		}()
	}

	if err := filepath.WalkDir(archiveRootPath, twCollector.treeWalkerBuilder(archiveRootPath)); err != nil {
		panic(err)
	}

	twCollector.close()
	wg.Wait()

	export := twCollector.finish()

	export.Commit = archiveCommit

	if err := outputConstants(&export, constantOutputPath); err != nil {
		panic(err)
	}
}

func outputConstants(export *constantfetch.BTFHubConstants, outputPath string) error {
	fmt.Printf("%d kernels\n", len(export.Kernels))
	fmt.Printf("%d unique constants\n", len(export.Constants))

	output, err := json.MarshalIndent(export, "", "\t")
	if err != nil {
		return err
	}

	return os.WriteFile(outputPath, output, 0644)
}

func combineConstantFiles(archiveRootPath string) (constantfetch.BTFHubConstants, error) {
	files := make([]constantfetch.BTFHubConstants, 0)

	err := filepath.WalkDir(archiveRootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".json") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var constants constantfetch.BTFHubConstants
		if err := json.Unmarshal(data, &constants); err != nil {
			return err
		}

		files = append(files, constants)

		return nil
	})
	if err != nil {
		return constantfetch.BTFHubConstants{}, err
	}

	if len(files) == 0 {
		return constantfetch.BTFHubConstants{}, errors.New("no json file found")
	}

	lastCommit := ""
	for _, file := range files {
		if lastCommit != "" && file.Commit != lastCommit {
			return constantfetch.BTFHubConstants{}, errors.New("multiple different commits in constant files")
		}
	}

	res := constantfetch.BTFHubConstants{
		Commit: lastCommit,
	}

	for _, file := range files {
		offset := len(res.Constants)
		res.Constants = append(res.Constants, file.Constants...)

		for _, kernel := range file.Kernels {
			kernel.ConstantsIndex += offset
			res.Kernels = append(res.Kernels, kernel)
		}
	}

	return res, nil
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
	sync.Mutex

	counter   int
	results   []extractionResult
	cache     map[string]map[string]uint64
	queryChan chan extractionQuery
}

func newTreeWalkCollector(preAllocHint int) *treeWalkCollector {
	return &treeWalkCollector{
		counter:   0,
		results:   make([]extractionResult, 0, preAllocHint),
		cache:     make(map[string]map[string]uint64),
		queryChan: make(chan extractionQuery),
	}
}

func (c *treeWalkCollector) close() {
	close(c.queryChan)
}

func (c *treeWalkCollector) finish() constantfetch.BTFHubConstants {
	slices.SortFunc(c.results, func(a extractionResult, b extractionResult) int {
		return cmp.Compare(a.counter, b.counter)
	})

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

		pathSuffix := strings.TrimPrefix(path, prefix)

		btfParts := strings.Split(pathSuffix, "/")
		if len(btfParts) < 4 {
			return fmt.Errorf("file has wrong format: %s", pathSuffix)
		}

		distribution := btfParts[len(btfParts)-4]
		distribVersion := btfParts[len(btfParts)-3]
		arch := btfParts[len(btfParts)-2]
		unameRelease := strings.TrimSuffix(btfParts[len(btfParts)-1], ".btf.tar.xz")

		c.queryChan <- extractionQuery{
			counter:        c.counter,
			path:           path,
			distribution:   distribution,
			distribVersion: distribVersion,
			arch:           arch,
			unameRelease:   unameRelease,
		}

		c.counter++
		return nil
	}
}

func (c *treeWalkCollector) extractor() error {
	for query := range c.queryChan {
		fmt.Println(query.counter, query.path)

		constants, err := c.extractConstantsFromBTF(query.path, query.distribution, query.distribVersion)
		if err != nil {
			return fmt.Errorf("failed to extract constants from `%s`: %v", query.path, err)
		}

		res := extractionResult{
			counter:        query.counter,
			distribution:   query.distribution,
			distribVersion: query.distribVersion,
			arch:           query.arch,
			unameRelease:   query.unameRelease,
			constants:      constants,
		}

		c.Lock()
		c.results = append(c.results, res)
		c.Unlock()
	}
	return nil
}

type extractionQuery struct {
	counter        int
	path           string
	distribution   string
	distribVersion string
	arch           string
	unameRelease   string
}

type extractionResult struct {
	counter        int
	distribution   string
	distribVersion string
	arch           string
	unameRelease   string
	constants      map[string]uint64
}

func (c *treeWalkCollector) getCacheEntry(cacheKey string) (map[string]uint64, bool) {
	c.Lock()
	defer c.Unlock()
	val, ok := c.cache[cacheKey]
	return val, ok
}

func (c *treeWalkCollector) extractConstantsFromBTF(archivePath, distribution, distribVersion string) (map[string]uint64, error) {
	btfContent, err := createBTFReaderFromTarball(archivePath)
	if err != nil {
		return nil, err
	}

	cacheKey := computeCacheKey(btfContent)
	if constants, ok := c.getCacheEntry(cacheKey); ok {
		return constants, nil
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

	btfReader := bytes.NewReader(btfContent)
	fetcher, err := constantfetch.NewBTFConstantFetcherFromReader(btfReader)
	if err != nil {
		return nil, err
	}

	probe.AppendProbeRequestsToFetcher(fetcher, kv)
	constants, err := fetcher.FinishAndGetResults()
	if err != nil {
		return nil, err
	}

	c.Lock()
	c.cache[cacheKey] = constants
	c.Unlock()
	return constants, nil
}

func createBTFReaderFromTarball(archivePath string) ([]byte, error) {
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
				btfBuffer.Grow(int(hdr.Size))
				if _, err := io.Copy(btfBuffer, tarReader); err != nil {
					return nil, fmt.Errorf("failed to uncompress file %s: %w", hdr.Name, err)
				}
				break outer
			}
		}
	}

	return btfBuffer.Bytes(), nil
}

func computeCacheKey(b []byte) string {
	h := sha256.New()
	h.Write(b)
	return fmt.Sprintf("%x", h.Sum(nil))
}
