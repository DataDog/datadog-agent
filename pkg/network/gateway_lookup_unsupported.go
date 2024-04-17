// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package network

import (
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// UnsupportedGatewayLookup is a no-op gateway lookup
// for operating systems that don't support it
type UnsupportedGatewayLookup struct{}

// NewGatewayLookup returns nil
func NewGatewayLookup() *UnsupportedGatewayLookup {
	return nil
}

// Lookup is a no-op
func (u *UnsupportedGatewayLookup) Lookup(cs *ConnectionStats) *Via {
	return nil
}

// LookupWithIPs is a no-op
func (u *UnsupportedGatewayLookup) LookupWithIPs(source util.Address, dest util.Address, netns uint32) *Via {
	return nil
}

// Close is a no-op
func (u *UnsupportedGatewayLookup) Close() {}
