// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package dns

import (
	"github.com/DataDog/datadog-agent/pkg/network/config"
)

// NewReverseDNS starts snooping on DNS traffic to allow IP -> domain reverse resolution
func NewReverseDNS(cfg *config.Config) (ReverseDNS, error) {
	packetSrc, err := newWindowsPacketSource()
	if err != nil {
		return nil, err
	}
	return newSocketFilterSnooper(cfg, packetSrc)
}
