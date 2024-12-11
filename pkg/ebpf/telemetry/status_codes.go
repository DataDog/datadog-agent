// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package telemetry

// BTFResult enumerates BTF loading success & failure modes
type BTFResult int

const (
	//SuccessCustomBTF metric is used to count custom BTF usages
	SuccessCustomBTF BTFResult = 0
	//SuccessEmbeddedBTF metric is used to count embedded minimized BTFs
	SuccessEmbeddedBTF BTFResult = 1
	//SuccessDefaultBTF metric is used to count built-in BTF file which is included on new kernel versions
	SuccessDefaultBTF BTFResult = 2
	//BtfNotFound returned when btf is not found in any of the expected locations
	BtfNotFound BTFResult = 3
)

// COREResult enumerates CO-RE success & failure modes
type COREResult int

const (
	// BTFResult comes beforehand

	//AssetReadError returned upon failure to read the ebpf program object file
	AssetReadError COREResult = 4
	//VerifierError returned when ebpf verifier rejects loading the CO-RE ebpf program
	VerifierError COREResult = 5
	//LoaderError returned upon failure to load the eBPF program, but not due to verifier rejection
	LoaderError COREResult = 6
)
