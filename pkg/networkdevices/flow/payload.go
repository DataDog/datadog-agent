// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flow

// DeviceFlow contains network devices flows
type DeviceFlow struct {
	SrcAddr         string `json:"src_addr"`
	DstAddr         string `json:"dst_addr"`
	SamplerAddr     string `json:"sampler_addr"`
	FlowType        string `json:"flow_type"`
	Proto           uint32 `json:"proto"`
	InputInterface  uint32 `json:"input_interface"`
	OutputInterface uint32 `json:"output_interface"`
	Direction       uint32 `json:"direction"`
	Bytes           uint64 `json:"bytes"`
	Packets         uint64 `json:"packets"`
}
