// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Contains BSD-2-Clause code (c) 2015-present Andrea Barberio

package net

// IPProto is the IP protocol type
type IPProto int

// a few common IANA protocol numbers
var (
	ProtoICMP   IPProto = 1
	ProtoTCP    IPProto = 6
	ProtoUDP    IPProto = 17
	ProtoICMPv6 IPProto = 58
)
