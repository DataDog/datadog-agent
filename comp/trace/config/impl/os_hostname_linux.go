// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && !serverless

package configimpl

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/system"
)

// isOsHostnameUsable mirrors the same check in pkg/util/hostname/os_hostname_linux.go.
// We do not import that package directly because pkg/util/hostname/common.go pulls in
// cloud-provider clients (Azure, GCE, EC2, Docker, Kubernetes) as transitive dependencies,
// which would add ~2 MiB to the trace-agent binary for a one-line call.
// The hostname_trust_uts_namespace config shortcut from the original is intentionally
// omitted here: pkg/config/setup is depguard-banned inside comp/, and users who need
// to override hostname resolution can set DD_HOSTNAME explicitly.
func isOsHostnameUsable() bool {
	selfUTSInode, err := system.GetProcessNamespaceInode("/proc", "self", "uts")
	if err != nil {
		log.Debug("Unable to get self UTS inode")
		return true
	}

	hostUTS := system.IsProcessHostUTSNamespace("/proc", selfUTSInode)
	if hostUTS == nil {
		log.Debug("Unable to compare self UTS inode to host UTS inode")
		return true
	}

	return *hostUTS
}
