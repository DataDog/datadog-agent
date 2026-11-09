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
	"golang.org/x/sys/unix"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
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

var tpBTFRejections = telemetryimpl.GetCompatComponent().NewCounter("network_tracer", "tp_btf_rejections",
	[]string{"program"}, "Counter measuring tp_btf programs the verifier rejected before falling back to the raw tracepoint")

// RetryWithoutTPBTF calls load, and calls it once more if the verifier rejected progName, a tp_btf
// net_dev_queue program, with NetDevQueueTPBTFRejected set so the retry selects the raw tracepoint.
// Call it inside the LoadCOREAsset callback so a recovered rejection is not reported as a CO-RE failure.
func RetryWithoutTPBTF(progName string, load func() error) error {
	err := load()
	ve, ok := tpBTFRejection(err, progName)
	if !ok || !NetDevQueueTPBTFRejected.CompareAndSwap(false, true) {
		return err
	}
	tpBTFRejections.Inc(progName)
	log.Warnf("verifier rejected %s, retrying with the raw tracepoint: %s", progName, ve)
	log.Debugf("%s verifier log:\n%+v", progName, ve)
	return load()
}

// tpBTFRejection returns the verifier error if err is the verifier rejecting progName. Other errnos
// that cilium/ebpf wraps in a VerifierError, such as ENOMEM, EPERM, or E2BIG, are not specific to
// tp_btf, so they would recur on a retry.
func tpBTFRejection(err error, progName string) (*cebpf.VerifierError, bool) {
	var ve *cebpf.VerifierError
	if err == nil || !errors.As(err, &ve) {
		return nil, false
	}
	if !errors.Is(ve.Cause, unix.EACCES) && !errors.Is(ve.Cause, unix.EINVAL) {
		return nil, false
	}
	// cilium/ebpf prefixes collection load errors with the program name
	return ve, strings.Contains(err.Error(), "program "+progName+":")
}
