// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package hostname

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/system"
)

// isOSHostnameUsable returns `false` if it has the certainty that the agent is running
// in a non-root UTS namespace because in that case, the OS hostname characterizes the
// identity of the agent container and not the one of the nodes it is running on.
func isOSHostnameUsable(ctx context.Context) bool {
	if config.Datadog.GetBool("hostname_trust_uts_namespace") {
		return true
	}

	selfUTSInode, err := system.GetProcessNamespaceInode("/proc", "self", "uts")
	if err != nil {
		// If we are not able to gather our own UTS Inode, in doubt, return true.
		log.Debug("Unable to get self UTS inode")
		return true
	}

	hostUTS := system.IsProcessHostUTSNamespace("/proc", selfUTSInode)
	if hostUTS == nil {
		// Similarly, if we were not able to compare, return true
		log.Debug("Unable to compare self UTS inode to host UTS inode")
		return true
	}

	return *hostUTS
}
