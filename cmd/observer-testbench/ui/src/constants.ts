// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

<<<<<<<< HEAD:cmd/observer-testbench/ui/src/constants.ts
/** Tag keys surfaced in filter chips and anomaly list headers. */
export const MAIN_TAG_FILTER_KEYS = new Set(['anomaly_type', 'status', 'service', 'host', 'detector', 'pod_name', 'container_name']);
========
//go:build linux

// Package types is DNFv2 types
package types

// Checksum is a DNFv2 checksum
type Checksum struct {
	Hash string `xml:",chardata"`
	Type string `xml:"type,attr"`
}
>>>>>>>> main:pkg/util/kernel/headers/download/rpm/dnfv2/types/checksum.go
