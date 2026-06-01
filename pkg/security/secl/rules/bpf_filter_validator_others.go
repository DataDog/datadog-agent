// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package rules

import "errors"

func init() {
	DefaultValidateBPFFilter = validateNetworkFilterBPFFilterUnsupported
}

func validateNetworkFilterBPFFilterUnsupported(_ string) error {
	return errors.New("BPF Filters are supported on this platform")
}
