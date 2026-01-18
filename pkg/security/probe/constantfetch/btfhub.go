// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package constantfetch holds constantfetch related files
package constantfetch

import (
	_ "embed" // for go:embed
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
)

// BTFHubConstantFetcher is a constant fetcher based on BTFHub constants
type BTFHubConstantFetcher struct {
	currentKernelInfos *kernelInfos
	inStore            map[string]uint64
	requests           []string
}

var archMapping = map[string]string{
	"amd64": "x86_64",
	"arm64": "arm64",
}

func (f *BTFHubConstantFetcher) fillStore() error {
	if len(f.inStore) != 0 {
		return nil
	}

	var constantsInfos BTFHubConstants
	if err := json.Unmarshal(btfhubConstants, &constantsInfos); err != nil {
		return err
	}

	for _, kernel := range constantsInfos.Kernels {
		if kernel.Distribution == f.currentKernelInfos.distribution && kernel.DistribVersion == f.currentKernelInfos.distribVersion && kernel.Arch == f.currentKernelInfos.arch && kernel.UnameRelease == f.currentKernelInfos.unameRelease {
			f.inStore = constantsInfos.Constants[kernel.ConstantsIndex]
			break
		}
	}

	return nil
}

// NewBTFHubConstantFetcher returns a new BTFHubConstantFetcher
func NewBTFHubConstantFetcher(kv *kernel.Version) (*BTFHubConstantFetcher, error) {
	currentKernelInfos, err := newKernelInfos(kv)
	if err != nil {
		return nil, fmt.Errorf("failed to collect current kernel infos: %w", err)
	}

	return &BTFHubConstantFetcher{
		currentKernelInfos: currentKernelInfos,
		inStore:            make(map[string]uint64),
	}, nil
}

func (f *BTFHubConstantFetcher) String() string {
	return "btfhub"
}

// AppendSizeofRequest appends a sizeof request
func (f *BTFHubConstantFetcher) AppendSizeofRequest(id, _ string) {
	f.requests = append(f.requests, id)
}

// AppendOffsetofRequestWithFallbacks appends an offset request
func (f *BTFHubConstantFetcher) AppendOffsetofRequestWithFallbacks(id string, _ ...TypeFieldPair) {
	f.requests = append(f.requests, id)
}

// FinishAndGetResults returns the results
func (f *BTFHubConstantFetcher) FinishAndGetResults() (map[string]uint64, error) {
	if len(f.requests) == 0 {
		return nil, nil
	}

	if err := f.fillStore(); err != nil {
		return nil, err
	}

	res := make(map[string]uint64)
	for _, id := range f.requests {
		if value, ok := f.inStore[id]; ok {
			res[id] = value
		} else {
			res[id] = ErrorSentinel
		}
	}

	return res, nil
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
		return nil, errors.New("failed to collect os-release ID")
	}

	version, ok := kv.OsRelease["VERSION_ID"]
	if !ok {
		return nil, errors.New("failed to collect os-release VERSION_ID")
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
		return nil, errors.New("failed to map runtime arch to btf arch")
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
