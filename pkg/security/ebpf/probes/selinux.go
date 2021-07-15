// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probes

import (
	"github.com/DataDog/ebpf/manager"
)

// selinuxProbes holds the list of probes used to track fs write events
var selinuxProbes = []*manager.Probe{
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/sel_write_disable",
	},
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/sel_write_enforce",
	},
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/sel_write_bool",
	},
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/sel_commit_bools_write",
	},
}

func getSELinuxProbes() []*manager.Probe {
	return selinuxProbes
}
