// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metadata

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

func TestProcessMetadataProvider_ParseMetadata(t *testing.T) {
	pmp := NewProcessMetadataProvider()

	pmp.Extract(processData())
}

func processData() map[int32]*procutil.Process {
	return map[int32]*procutil.Process{
		1: {
			Pid: 1,
			Cmdline: []string{
				"/sbin/init",
			},
		},
		23211: {
			Pid: 23211,
			Cmdline: []string{
				"/usr/bin/docker-proxy",
				"-proto",
				"tcp",
				"-host-ip",
				"0.0.0.0",
				"-host-port",
				"32769",
				"-container-ip",
				"172.17.0.2",
				"-container-port",
				"6379",
			},
		},
	}
}
