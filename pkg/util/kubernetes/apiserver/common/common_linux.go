// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubeapiserver && linux

package common

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/system"
)

// GetSelfPodName returns hostname from DD_POD_NAME in helm chart, if not found, use os.hostname
func GetSelfPodName() (string, error) {
	if podName, ok := os.LookupEnv("DD_POD_NAME"); ok {
		return podName, nil
	}

	selfUTSInode, err := system.GetProcessNamespaceInode("/proc", "self", "uts")
	if err != nil {
		// If we are not able to gather our own UTS Inode, in doubt, authorize fallback to `os.Hostname()`
		log.Warnf("Unable to get self UTS inode")
		return os.Hostname()
	}

	hostUTS := system.IsProcessHostUTSNamespace("/proc", selfUTSInode)
	if hostUTS == nil {
		// In doubt, authorize fallback to `os.Hostname()`
		return os.Hostname()
	}

	if *hostUTS {
		return "", fmt.Errorf("DD_POD_NAME is not set and running in host UTS namespace; cannot reliably determine self pod name")
	}

	return os.Hostname()
}
