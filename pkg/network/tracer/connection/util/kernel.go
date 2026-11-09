// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package util

import (
	"errors"
	"strings"
	"sync/atomic"

	cebpf "github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ksyms"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// HasTCPSendPage checks if the kernel has the tcp_sendpage function.
// After kernel 6.5.0, tcp_sendpage and udp_sendpage are removed.
// We used to only check for kv < 6.5.0 here - however, OpenSUSE 15.6 backported
// this change into 6.4.0 to pick up a CVE so the version number is not reliable.
// Instead, we directly check if the function exists.
func HasTCPSendPage(kv kernel.Version) bool {
	missing, err := ksyms.VerifyKernelFuncs("tcp_sendpage")
	if err == nil {
		return len(missing) == 0
	}

	log.Debugf("unable to determine whether tcp_sendpage exists, using kernel version instead: %s", err)

	kv650 := kernel.VersionCode(6, 5, 0)
	return kv < kv650
}

// NetDevQueueTPBTFRejected is set once the verifier rejects a tp_btf net_dev_queue program, so that
// later tracer loads, including a fallback to another tracer, select the raw tracepoint instead.
var NetDevQueueTPBTFRejected atomic.Bool

// ShouldRetryWithoutTPBTF reports whether err is the verifier rejecting progName, a tp_btf
// net_dev_queue program. If so, it sets NetDevQueueTPBTFRejected so the retry excludes it.
func ShouldRetryWithoutTPBTF(err error, progName string) bool {
	var ve *cebpf.VerifierError
	if err == nil || !errors.As(err, &ve) || !strings.Contains(err.Error(), "program "+progName+":") {
		return false
	}
	return NetDevQueueTPBTFRejected.CompareAndSwap(false, true)
}
