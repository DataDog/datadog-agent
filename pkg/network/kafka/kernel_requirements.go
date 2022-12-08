// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package kafka

import (
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

var MinimumKernelVersion kernel.Version

func init() {
	MinimumKernelVersion = kernel.VersionCode(5, 4, 0)
}
