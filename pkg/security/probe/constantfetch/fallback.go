// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package constantfetch

import (
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
)

// FallbackConstantFetcher is a constant fetcher that uses the old fallback
// heuristics to fetch constants
type FallbackConstantFetcher struct {
	kernelVersion *kernel.Version
	res           map[string]uint64
}

// NewFallbackConstantFetcher returns a new FallbackConstantFetcher
func NewFallbackConstantFetcher(kv *kernel.Version) *FallbackConstantFetcher {
	return &FallbackConstantFetcher{
		kernelVersion: kv,
		res:           make(map[string]uint64),
	}
}

func (f *FallbackConstantFetcher) appendRequest(id string) {
	var value = errorSentinel
	switch id {
	case "sizeof_inode":
		value = getSizeOfStructInode(f.kernelVersion)
	case "sb_magic_offset":
		value = getSuperBlockMagicOffset(f.kernelVersion)
	case "tty_offset":
		value = getSignalTTYOffset(f.kernelVersion)
	case "tty_name_offset":
		value = getTTYNameOffset(f.kernelVersion)
	}
	f.res[id] = value
}

// AppendSizeofRequest appends a sizeof request
func (f *FallbackConstantFetcher) AppendSizeofRequest(id, typeName, headerName string) {
	f.appendRequest(id)
}

// AppendOffsetofRequest appends an offset request
func (f *FallbackConstantFetcher) AppendOffsetofRequest(id, typeName, fieldName, headerName string) {
	f.appendRequest(id)
}

// FinishAndGetResults returns the results
func (f *FallbackConstantFetcher) FinishAndGetResults() (map[string]uint64, error) {
	return f.res, nil
}

func getSizeOfStructInode(kv *kernel.Version) uint64 {
	sizeOf := uint64(600)

	switch {
	case kv.IsRH7Kernel():
		sizeOf = 584
	case kv.IsRH8Kernel():
		sizeOf = 648
	case kv.IsSLES12Kernel():
		sizeOf = 560
	case kv.IsSLES15Kernel():
		sizeOf = 592
	case kv.IsOracleUEKKernel():
		sizeOf = 632
	case kv.Code != 0 && kv.Code < kernel.Kernel4_16:
		sizeOf = 608
	case kernel.Kernel5_0 <= kv.Code && kv.Code < kernel.Kernel5_1:
		sizeOf = 584
	}

	return sizeOf
}

func getSuperBlockMagicOffset(kv *kernel.Version) uint64 {
	sizeOf := uint64(96)

	if kv.IsRH7Kernel() {
		sizeOf = 88
	}

	return sizeOf
}

func getSignalTTYOffset(kv *kernel.Version) uint64 {
	ttyOffset := uint64(400)

	switch {
	case kv.IsRH7Kernel():
		ttyOffset = 416
	case kv.IsRH8Kernel():
		ttyOffset = 392
	case kv.IsSLES12Kernel():
		ttyOffset = 376
	case kv.IsSLES15Kernel():
		ttyOffset = 408
	case kv.Code != 0 && kv.Code < kernel.Kernel5_3:
		ttyOffset = 368
	}

	return ttyOffset
}

func getTTYNameOffset(kv *kernel.Version) uint64 {
	nameOffset := uint64(368)

	switch {
	case kv.IsRH7Kernel():
		nameOffset = 312
	case kv.IsRH8Kernel():
		nameOffset = 368
	case kv.IsSLES12Kernel():
		nameOffset = 368
	case kv.IsSLES15Kernel():
		nameOffset = 368
	case kv.Code != 0 && kv.Code < kernel.Kernel5_3:
		nameOffset = 368
	}

	return nameOffset
}
