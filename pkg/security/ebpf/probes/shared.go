// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probes

import "github.com/DataDog/ebpf/manager"

// sharedProbes is the list of probes that are shared across multiple events
var sharedProbes = []*manager.Probe{
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/filename_create",
	},
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/mnt_want_write",
	},
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/mnt_want_write_file",
	},
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/mnt_want_write_file_path",
	},
}
