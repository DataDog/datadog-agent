// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package constantfetch

import (
	_ "embed"
	"encoding/json"
	"errors"
	"runtime"

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

var idToDistribMapping = map[string]string{
	"ubuntu": "ubuntu",
	"debian": "debian",
	"amzn":   "amzn",
	"centos": "centos",
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

	currentKernelInfos, ok := newKernelInfos(kv)
	if !ok {
		return nil, errors.New("failed to collect current kernel infos")
	}

	var constantsInfos []BTFHubConstantsInfo
	if err := json.Unmarshal(btfhubConstants, &constantsInfos); err != nil {
		return nil, err
	}

	for _, ci := range constantsInfos {
		if ci.Distribution == currentKernelInfos.distribution && ci.DistribVersion == currentKernelInfos.distribVersion && ci.Arch == currentKernelInfos.arch && ci.UnameRelease == currentKernelInfos.unameRelease {
			fetcher.inStore = ci.Constants
			break
		}
	}

	return fetcher, nil
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

func newKernelInfos(kv *kernel.Version) (*kernelInfos, bool) {
	releaseID, ok := kv.OsRelease["ID"]
	if !ok {
		return nil, false
	}

	distribution, ok := idToDistribMapping[releaseID]
	if !ok {
		return nil, false
	}

	version, ok := kv.OsRelease["VERSION_ID"]
	if !ok {
		return nil, false
	}

	arch, ok := archMapping[runtime.GOARCH]
	if !ok {
		return nil, false
	}

	return &kernelInfos{
		distribution:   distribution,
		distribVersion: version,
		arch:           arch,
		unameRelease:   kv.UnameRelease,
	}, true
}

// BTFHubConstantsInfo represents all the information required for identifying
// a unique btf file from BTFHub
type BTFHubConstantsInfo struct {
	Distribution   string            `json:"distrib"`
	DistribVersion string            `json:"version"`
	Arch           string            `json:"arch"`
	UnameRelease   string            `json:"uname_release"`
	Constants      map[string]uint64 `json:"constants"`
}
