// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package ssluprobes

import (
	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
)

// OpenSSLUProbes is the list of uprobes installed by the ssluprobes attacher
var OpenSSLUProbes = []probes.ProbeFuncName{
	probes.SSLDoHandshakeProbe,
	probes.SSLDoHandshakeRetprobe,
	probes.SSLReadProbe,
	probes.SSLReadRetprobe,
	probes.SSLReadExProbe,
	probes.SSLReadExRetprobe,
	probes.SSLWriteProbe,
	probes.SSLWriteRetprobe,
	probes.SSLWriteExProbe,
	probes.SSLWriteExRetprobe,
	probes.I2DX509Probe,
	probes.I2DX509Retprobe,
}

// GetSchedExitProbeSSL gets the tracepoint probe used to clean up dead PIDs
func GetSchedExitProbeSSL() *manager.Probe {
	return &manager.Probe{
		ProbeIdentificationPair: IDPairFromFuncName(probes.RawTracepointSchedProcessExit),
		TracepointName:          "sched_process_exit",
	}
}

// CNMModuleName is the name of the CNM module, which is used for attaching uprobes
const CNMModuleName = "cnm"

// CNMTLSAttacherName is the name of the CNM TLS uprobe attacher
const CNMTLSAttacherName = "cnm-ssl"

const probeUID = "ssl-certs"

// IDPairFromFuncName creates an ebpf manager ProbeIdentificationPair for the given function
func IDPairFromFuncName(funcName probes.ProbeFuncName) manager.ProbeIdentificationPair {
	return manager.ProbeIdentificationPair{
		UID:          probeUID,
		EBPFFuncName: funcName,
	}
}
