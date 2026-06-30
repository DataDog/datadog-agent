// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !(linux && pcap && cgo)

package rules

import "errors"

func init() {
	DefaultValidateBPFFilter = validateNetworkFilterBPFFilterFallback
}

func validateNetworkFilterBPFFilterFallback(_ string) error {
	return errors.New("BPF Filters are not supported")
}
