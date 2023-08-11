// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cgroups

const (
	sampleCgroupV2IOStat = `259:0 rbytes=278528 wbytes=11623899136 rios=6 wios=2744940 dbytes=0 dios=0
8:16 rbytes=278528 wbytes=11623899136 rios=6 wios=2744940 dbytes=0 dios=0`
	sampleCgroupV2IOMax     = "8:16 rbps=2097152 wbps=max riops=max wiops=120"
	sampleCroupV2IOPressure = `some avg10=0.00 avg60=0.00 avg300=0.00 total=0
full avg10=0.00 avg60=0.00 avg300=0.00 total=0`
)

func createCgroupV2FakeIOFiles(cfs *cgroupMemoryFS, cg *cgroupV2) {
	cfs.setCgroupV2File(cg, "io.stat", sampleCgroupV2IOStat)
	cfs.setCgroupV2File(cg, "io.max", sampleCgroupV2IOMax)
	cfs.setCgroupV2File(cg, "io.pressure", sampleCroupV2IOPressure)
}
