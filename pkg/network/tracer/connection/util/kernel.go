// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package util

import (
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// HasTCPSendPage checks if the kernel has the tcp_sendpage function.
// After kernel 6.5.0, tcp_sendpage and udp_sendpage are removed.
// We used to only check for kv < 6.5.0 here - however, OpenSUSE 15.6 backported
// this change into 6.4.0 to pick up a CVE so the version number is not reliable.
// Instead, we directly check if the function exists.
func HasTCPSendPage(kv kernel.Version) bool {
	missing, err := ebpf.VerifyKernelFuncs("tcp_sendpage")
	if err == nil {
		return len(missing) == 0
	}

	log.Debugf("unable to determine whether tcp_sendpage exists, using kernel version instead: %s", err)

	kv650 := kernel.VersionCode(6, 5, 0)
	return kv < kv650
}
