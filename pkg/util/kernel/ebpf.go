// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kernel

import (
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/fargate"
	"github.com/DataDog/datadog-agent/pkg/util/funcs"
)

// see https://github.com/iovisor/bcc/blob/master/docs/kernel-versions.md
var minKernelVersionKprobeSupported = VersionCode(4, 1, 0)

// IsEbpfSupported returns `true` if eBPF is supported on this platform
var IsEbpfSupported = funcs.MemoizeNoError(func() bool {
	if fargate.IsFargateInstance() {
		return false
	}

	family, err := Family()
	if err != nil {
		return false
	}

	if family == "rhel" {
		pv, err := PlatformVersion()
		if err != nil {
			return false
		}

		if pvs := strings.SplitN(pv, ".", 3); len(pvs) > 1 {
			major, _ := strconv.Atoi(pvs[0])
			minor, _ := strconv.Atoi(pvs[1])
			return major >= 7 && minor >= 6
		}

		return false
	}

	kv := MustHostVersion()
	return kv >= minKernelVersionKprobeSupported
})
