// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package constantfetch

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
)

//go:embed btfhub/constants.json
var btfhubConstants []byte

// BTFHubConstantFetcher is a constant fetcher based on BTFHub constants
type BTFHubConstantFetcher struct {
	kernelVersion *kernel.Version
	inStore       map[string]uint64
	res           map[string]uint64
}

var archMapping = map[string]string{
	"amd64": "x86_64",
	"arm64": "arm64",
}

// NewBTFHubConstantFetcher returns a new BTFHubConstantFetcher
func NewBTFHubConstantFetcher(kv *kernel.Version) (*BTFHubConstantFetcher, error) {
	fetcher := &BTFHubConstantFetcher{
		kernelVersion: kv,
		inStore:       make(map[string]uint64),
		res:           make(map[string]uint64),
	}

	currentKernelInfos, err := newKernelInfos(kv)
	if err != nil {
		return nil, fmt.Errorf("failed to collect current kernel infos: %w", err)
	}

	var constantsInfos BTFHubConstants
	if err := json.Unmarshal(btfhubConstants, &constantsInfos); err != nil {
		return nil, err
	}

	for _, kernel := range constantsInfos.Kernels {
		if kernel.Distribution == currentKernelInfos.distribution && kernel.DistribVersion == currentKernelInfos.distribVersion && kernel.Arch == currentKernelInfos.arch && kernel.UnameRelease == currentKernelInfos.unameRelease {
			fetcher.inStore = constantsInfos.Constants[kernel.ConstantsIndex]
			break
		}
	}

	return fetcher, nil
}

func (f *BTFHubConstantFetcher) String() string {
	return "btfhub"
}

// HasConstantsInStore returns true if there is constants in store in BTFHub
func (f *BTFHubConstantFetcher) HasConstantsInStore() bool {
	return len(f.inStore) != 0
}

func (f *BTFHubConstantFetcher) appendRequest(id string) {
	if value, ok := f.inStore[id]; ok {
		f.res[id] = value
	} else {
		f.res[id] = ErrorSentinel
	}
}

// AppendSizeofRequest appends a sizeof request
func (f *BTFHubConstantFetcher) AppendSizeofRequest(id, typeName, headerName string) {
	f.appendRequest(id)
}

// AppendOffsetofRequest appends an offset request
func (f *BTFHubConstantFetcher) AppendOffsetofRequest(id, typeName, fieldName, headerName string) {
	f.appendRequest(id)
}

// FinishAndGetResults returns the results
func (f *BTFHubConstantFetcher) FinishAndGetResults() (map[string]uint64, error) {
	return f.res, nil
}

type kernelInfos struct {
	distribution   string
	distribVersion string
	arch           string
	unameRelease   string
}

func newKernelInfos(kv *kernel.Version) (*kernelInfos, error) {
	distribution, ok := kv.OsRelease["ID"]
	if !ok {
		return nil, fmt.Errorf("failed to collect os-release ID")
	}

	version, ok := kv.OsRelease["VERSION_ID"]
	if !ok {
		return nil, fmt.Errorf("failed to collect os-release VERSION_ID")
	}

	// HACK: fix mapping of version for oracle-linux and amazon linux 2018
	switch {
	case distribution == "ol" && strings.HasPrefix(version, "7."):
		version = "7"
	case distribution == "amzn" && strings.HasPrefix(version, "2018."):
		version = "2018"
	}

	arch, ok := archMapping[runtime.GOARCH]
	if !ok {
		return nil, fmt.Errorf("failed to map runtime arch to btf arch")
	}

	return &kernelInfos{
		distribution:   distribution,
		distribVersion: version,
		arch:           arch,
		unameRelease:   kv.UnameRelease,
	}, nil
}

// BTFHubConstants represents all the information required for identifying
// a unique btf file from BTFHub
type BTFHubConstants struct {
	Commit    string              `json:"commit"`
	Constants []map[string]uint64 `json:"constants"`
	Kernels   []BTFHubKernel      `json:"kernels"`
}

// BTFHubKernel represents all the information required for identifying
// a unique btf file from BTFHub
type BTFHubKernel struct {
	Distribution   string `json:"distrib"`
	DistribVersion string `json:"version"`
	Arch           string `json:"arch"`
	UnameRelease   string `json:"uname_release"`
	ConstantsIndex int    `json:"cindex"`
}
