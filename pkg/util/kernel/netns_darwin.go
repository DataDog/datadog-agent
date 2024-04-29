// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kernel

import (
	"fmt"

	"github.com/vishvananda/netns"
)

// GetNetNsInoFromPid gets the network namespace inode number for the given
// `pid`
func GetNetNsInoFromPid(_ string, _ int) (uint32, error) {
	return 0, fmt.Errorf("not supported")
}

// GetInoForNs gets the inode number for the given network namespace
func GetInoForNs(_ netns.NsHandle) (uint32, error) {
	return 0, fmt.Errorf("not supported")
}
