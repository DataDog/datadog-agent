// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(NET) Fix revive linter
package types

import "github.com/DataDog/datadog-agent/pkg/process/util"

// ConnectionKey represents a network four-tuple (source IP, destination IP, source port, destination port)
type ConnectionKey struct {
	SrcIPHigh uint64
	SrcIPLow  uint64

	DstIPHigh uint64
	DstIPLow  uint64

	// ports separated for alignment/size optimization
	SrcPort uint16
	DstPort uint16
}

// NewConnectionKey generates a new ConnectionKey
func NewConnectionKey(saddr, daddr util.Address, sport, dport uint16) ConnectionKey {
	saddrl, saddrh := util.ToLowHigh(saddr)
	daddrl, daddrh := util.ToLowHigh(daddr)
	return ConnectionKey{
		SrcIPHigh: saddrh,
		SrcIPLow:  saddrl,
		SrcPort:   sport,
		DstIPHigh: daddrh,
		DstIPLow:  daddrl,
		DstPort:   dport,
	}
}
