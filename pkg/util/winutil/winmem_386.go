// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package winutil

type performanceInformation struct {
	cb                uint32
	commitTotal       uint32
	commitLimit       uint32
	commitPeak        uint32
	physicalTotal     uint32
	physicalAvailable uint32
	systemCache       uint32
	kernelTotal       uint32
	kernelPaged       uint32
	kernelNonpaged    uint32
	pageSize          uint32
	handleCount       uint32
	processCount      uint32
	threadCount       uint32
}
