// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(NET) Fix revive linter
package types

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

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

// String returns a string representation of the ConnectionKey
func (c *ConnectionKey) String() string {
	return fmt.Sprintf(
		"[%v:%d ⇄ %v:%d]",
		util.FromLowHigh(c.SrcIPLow, c.SrcIPHigh),
		c.SrcPort,
		util.FromLowHigh(c.DstIPLow, c.DstIPHigh),
		c.DstPort,
	)
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
