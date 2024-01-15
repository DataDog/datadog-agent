// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package telemetry

// BTFResult enumerates BTF loading success & failure modes
type BTFResult int

const (
	SuccessCustomBTF   BTFResult = 0
	SuccessEmbeddedBTF BTFResult = 1
	SuccessDefaultBTF  BTFResult = 2
	BtfNotFound        BTFResult = 3
)

// COREResult enumerates CO-RE success & failure modes
type COREResult int

const (
	// BTFResult comes beforehand

	AssetReadError COREResult = 4
	VerifierError  COREResult = 5
	LoaderError    COREResult = 6
)
