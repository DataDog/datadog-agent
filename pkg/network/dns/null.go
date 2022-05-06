// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows || linux_bpf
// +build windows linux_bpf

package dns

import (
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// NewNullReverseDNS returns a dummy implementation of ReverseDNS
func NewNullReverseDNS() ReverseDNS {
	return nullReverseDNS{}
}

type nullReverseDNS struct{}

func (nullReverseDNS) Resolve(_ []util.Address) map[util.Address][]Hostname {
	return nil
}

func (nullReverseDNS) GetDNSStats() StatsByKeyByNameByType {
	return nil
}

func (nullReverseDNS) GetStats() map[string]int64 {
	return map[string]int64{
		"lookups":           0,
		"resolved":          0,
		"ips":               0,
		"added":             0,
		"expired":           0,
		"packets_received":  0,
		"packets_processed": 0,
		"packets_dropped":   0,
		"socket_polls":      0,
		"decoding_errors":   0,
	}
}

func (nullReverseDNS) Start() error {
	return nil
}

func (nullReverseDNS) Close() {}

var _ ReverseDNS = nullReverseDNS{}
