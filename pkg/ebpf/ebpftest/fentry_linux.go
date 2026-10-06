// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package ebpftest

import "github.com/DataDog/datadog-agent/pkg/ebpf/features"

// supportsFentry reports whether the runtime kernel can attach fentry programs
// without hitting the tasks_rcu_exit_srcu detach deadlock, probing
// features.SupportsFentry with the same function the production fentry tracers
// attach to.
func supportsFentry() bool {
	return features.SupportsFentry("tcp_recvmsg") == nil
}
