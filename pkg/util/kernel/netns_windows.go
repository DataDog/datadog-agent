// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kernel

import "fmt"

// GetNetNsInoFromPid gets the network namespace inode number for the given
// `pid`
func GetNetNsInoFromPid(procRoot string, pid int) (uint32, error) { //nolint:revive // TODO fix revive unused-parameter
	return 0, fmt.Errorf("not supported")
}
