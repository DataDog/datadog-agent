// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package ebpftest

import (
	"github.com/DataDog/datadog-agent/pkg/ebpf/features"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// supportsFentry reports whether the runtime kernel can attach fentry programs
// without hitting the tasks_rcu_exit_srcu detach deadlock, probing
// features.SupportsFentry with the same function the production fentry tracers
// attach to.
func supportsFentry() bool {
	// The probe loads a BPF program; on pre-5.11 kernels that memory is
	// charged against RLIMIT_MEMLOCK. Raise the (memoized) limit first so a
	// low inherited limit cannot fail the probe; TestBuildMode raises it
	// too late, after SupportedBuildModes has already been evaluated.
	if err := removeMemlock(); err != nil {
		log.Warnf("excluding fentry build mode: failed to raise memlock limit: %v", err)
		return false
	}
	if err := features.SupportsFentry("tcp_recvmsg"); err != nil {
		log.Warnf("excluding fentry build mode: %v", err)
		return false
	}
	return true
}
