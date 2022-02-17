// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flow

const (
	defaultPortNETFLOW = uint16(2055)
	defaultPortIPFIX   = uint16(4739)
	defaultPortSFLOW   = uint16(6343)
	defaultStopTimeout = 5
)

type FlowType string

const (
	IPFIX    FlowType = "ipfix"
	SFLOW             = "sflow"
	NETFLOW5          = "netflow5"
	NETFLOW9          = "netflow9"
)
