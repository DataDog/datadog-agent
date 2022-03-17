// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/probe/constantfetch"
	utilKernel "github.com/DataDog/datadog-agent/pkg/util/kernel"
	cbtf "github.com/paulcacheux/cilium-btf/fork/btf"
)

func main() {
	var archiveRootPath string
	var constantOutputPath string
	var sampling int

	flag.StringVar(&archiveRootPath, "archive-root", "", "Root path of BTFHub archive")
	flag.StringVar(&constantOutputPath, "output", "", "Output path for JSON constants")
	flag.IntVar(&sampling, "sampling", 1, "Sampling rate, take 1 over n elements")
	flag.Parse()

	twCollector := newTreeWalkCollector(sampling)

	if err := filepath.WalkDir(archiveRootPath, twCollector.treeWalkerBuilder(archiveRootPath)); err != nil {
		panic(err)
	}
	fmt.Printf("%d kernels\n", len(twCollector.kernels))
	fmt.Printf("%d unique constants\n", len(twCollector.constants))

	export := constantfetch.BTFHubConstants{
		Constants: twCollector.constants,
		Kernels:   twCollector.kernels,
	}

	output, err := json.MarshalIndent(export, "", "\t")
	if err != nil {
		panic(err)
	}

	if err := os.WriteFile(constantOutputPath, output, 0644); err != nil {
		panic(err)
	}
}

type treeWalkCollector struct {
	constants []map[string]uint64
	kernels   []constantfetch.BTFHubKernel
	counter   int
	sampling  int
}

func newTreeWalkCollector(sampling int) *treeWalkCollector {
	return &treeWalkCollector{
		constants: make([]map[string]uint64, 0),
		kernels:   make([]constantfetch.BTFHubKernel, 0),
		counter:   0,
		sampling:  sampling,
	}
}

func (c *treeWalkCollector) appendConstants(distrib, version, arch, unameRelease string, constants map[string]uint64) {
	index := -1
	for i, other := range c.constants {
		if reflect.DeepEqual(other, constants) {
			index = i
			break
		}
	}

	if index == -1 {
		index = len(c.constants)
		c.constants = append(c.constants, constants)
	}

	c.kernels = append(c.kernels, constantfetch.BTFHubKernel{
		Distribution:   distrib,
		DistribVersion: version,
		Arch:           arch,
		UnameRelease:   unameRelease,
		ConstantsIndex: index,
	})
}

func (c *treeWalkCollector) treeWalkerBuilder(prefix string) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, err error) error {
		c.counter++
		if c.counter != c.sampling {
			return nil
		}
		c.counter = 0

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
		if len(btfParts) != 4 {
			return fmt.Errorf("file has wront format: %s", pathSuffix)
		}

		distribution := btfParts[0]
		distribVersion := btfParts[1]
		arch := btfParts[2]
		unameRelease := strings.TrimSuffix(btfParts[3], ".btf.tar.xz")

		fmt.Println(path)

		constants, err := extractConstantsFromBTF(path, distribution, distribVersion)
		if err != nil {
			return err
		}

		c.appendConstants(distribution, distribVersion, arch, unameRelease, constants)
		return nil
	}
}

func extractConstantsFromBTF(archivePath, distribution, distribVersion string) (map[string]uint64, error) {
	tmpDir, err := os.MkdirTemp("", "extract-dir")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	extractCmd := exec.Command("tar", "xvf", archivePath, "-C", tmpDir)
	if err := extractCmd.Run(); err != nil {
		return nil, err
	}

	archiveFileName := path.Base(archivePath)
	btfFileName := strings.TrimSuffix(archiveFileName, ".tar.xz")
	btfPath := path.Join(tmpDir, btfFileName)

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

	fetcher := newConstantCollector(btfPath)

	return probe.GetOffsetConstantsFromFetcher(fetcher, kv)
}

type constantCollector struct {
	btfPath  string
	requests []constantRequest
}

func newConstantCollector(btfPath string) *constantCollector {
	return &constantCollector{
		btfPath:  btfPath,
		requests: make([]constantRequest, 0),
	}
}

type constantRequest struct {
	id                  string
	sizeof              bool
	typeName, fieldName string
}

func (cc *constantCollector) AppendSizeofRequest(id, typeName, headerName string) {
	cc.requests = append(cc.requests, constantRequest{
		id:       id,
		sizeof:   true,
		typeName: getActualTypeName(typeName),
	})
}

func (cc *constantCollector) AppendOffsetofRequest(id, typeName, fieldName, headerName string) {
	cc.requests = append(cc.requests, constantRequest{
		id:        id,
		sizeof:    false,
		typeName:  getActualTypeName(typeName),
		fieldName: fieldName,
	})
}

func (cc *constantCollector) FinishAndGetResults() (map[string]uint64, error) {
	return extractWithCiliumBTF(cc.btfPath, cc.requests)
}

func getActualTypeName(tn string) string {
	prefixes := []string{"struct", "enum"}
	for _, prefix := range prefixes {
		tn = strings.TrimPrefix(tn, prefix+" ")
	}
	return tn
}

func extractWithCiliumBTF(btfPath string, requests []constantRequest) (map[string]uint64, error) {
	btfFile, err := os.Open(btfPath)
	if err != nil {
		return nil, err
	}

	spec, err := cbtf.LoadSpecFromReader(btfFile)
	if err != nil {
		return nil, err
	}

	constants := make(map[string]uint64)

	for _, r := range requests {
		actualTy := getActualTypeName(r.typeName)
		types, err := spec.AnyTypesByName(actualTy)
		if err != nil {
			continue
		}

		// the spec can contain multiple types for the same name
		// we check that they all return the same value for the same request
		for _, ty := range types {
			value := runRequestOnBTFType(r, ty)
			if value != constantfetch.ErrorSentinel {
				if previous, ok := constants[r.id]; ok && previous != value {
					return nil, errors.New("mismatching values in multiple BTF types")
				}
				constants[r.id] = value
			}
		}
	}

	return constants, nil
}

func runRequestOnBTFType(r constantRequest, ty cbtf.Type) uint64 {
	sTy, ok := ty.(*cbtf.Struct)
	if !ok {
		return constantfetch.ErrorSentinel
	}

	if r.sizeof {
		return uint64(sTy.Size)
	}

	for _, m := range sTy.Members {
		if m.Name == r.fieldName {
			return uint64(m.OffsetBits) / 8
		}
	}

	return constantfetch.ErrorSentinel
}
