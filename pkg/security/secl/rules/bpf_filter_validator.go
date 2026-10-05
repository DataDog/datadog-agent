// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

// DefaultValidateBPFFilter validates network_filter BPF expressions.
// The implementation requires a Linux build with pcap and cgo enabled.
var DefaultValidateBPFFilter func(bpfFilter string) error

func validateBPFFilterWithDefault(opts PolicyLoaderOpts, bpfFilter string) error {
	validate := opts.ValidateBPFFilter
	if validate == nil {
		validate = DefaultValidateBPFFilter
	}
	if validate == nil {
		return nil
	}
	return validate(bpfFilter)
}
