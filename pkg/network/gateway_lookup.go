// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package network

import "github.com/DataDog/datadog-agent/pkg/process/util"

// GatewayLookup is an interface for performing gateway lookups
type GatewayLookup interface {
	Lookup(cs *ConnectionStats) *Via
	LookupWithIPs(source util.Address, dest util.Address, netns uint32) *Via
	Close()
}
