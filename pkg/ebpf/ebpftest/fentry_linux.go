// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package ebpftest

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/ebpf/features"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
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
	if isRHEL8() {
		log.Warn("excluding fentry build mode: RHEL 8 based distributions backport fentry support into " +
			"their 4.18 kernels, but gcc renames some network tracer attach targets as IPA optimization " +
			"clones (e.g. udp_send_skb -> udp_send_skb.isra.NN) which are absent from kernel BTF, " +
			"so the fentry tracers cannot load")
		return false
	}
	return true
}

// isRHEL8 reports whether the host runs a RHEL 8 based distribution (RHEL,
// CentOS, Rocky, AlmaLinux, ...). The runtime probe above succeeds on those
// systems because their 4.18 kernels backport fentry support together with
// native kernel BTF, but several of the network tracer's fentry attach targets
// (udp_send_skb, udp_v6_send_skb) are compiled as gcc IPA optimization clones
// (e.g. udp_send_skb.isra.52) and therefore have no BTF entries, so fentry
// tracers cannot attach to them.
func isRHEL8() bool {
	family, err := kernel.Family()
	if err != nil || family != "rhel" {
		return false
	}
	version, err := kernel.PlatformVersion()
	if err != nil {
		return false
	}
	major, _, _ := strings.Cut(version, ".")
	return major == "8"
}
