// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package system

// NetworkRoute holds one network destination subnet and it's linked interface name
type NetworkRoute struct {
	Interface string
	Subnet    uint64
	Gateway   uint64
	Mask      uint64
}
