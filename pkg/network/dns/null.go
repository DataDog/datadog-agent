// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows || linux_bpf

package dns

import (
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// NewNullReverseDNS returns a dummy implementation of ReverseDNS
func NewNullReverseDNS() ReverseDNS {
	return nullReverseDNS{}
}

type nullReverseDNS struct{}

func (d nullReverseDNS) WaitForDomain(_ string) error {
	return nil
}

func (nullReverseDNS) Resolve(_ map[util.Address]struct{}) map[util.Address][]Hostname {
	return nil
}

func (nullReverseDNS) GetDNSStats() StatsByKeyByNameByType {
	return nil
}

func (nullReverseDNS) Start() error {
	return nil
}

func (nullReverseDNS) Close() {}

var _ ReverseDNS = nullReverseDNS{}
