// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && bpf && nvml

// Methods of the user-space event pipeline that only the eBPF side (probe.go, consumer.go) calls. They are in a
// bpf-tagged file so the non-bpf build, which runs the user-space unit tests, doesn't carry unused code.

package gpu

import (
	"github.com/DataDog/datadog-agent/pkg/gpu/config"
	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/probe/ebpf"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func withConfig(config *config.Config) systemContextOption {
	return func(opts *systemContextOptions) {
		opts.config = config
	}
}

func withDeviceCache(deviceCache ddnvml.DeviceCache) systemContextOption {
	return func(opts *systemContextOptions) {
		opts.deviceCache = deviceCache
	}
}

func (sh *StreamHandler) handleSync(event *gpuebpf.CudaSync) {
	sh.lastEventKtimeNs = event.Header.Ktime_ns

	// TODO: Worry about concurrent calls to this?
	sh.markSynchronization(event.Header.Ktime_ns)
}

func (sc *streamCollection) markProcessStreamsAsEnded(pid uint32) {
	for _, handler := range sc.allStreams() {
		if handler.metadata.pid == pid {
			log.Debugf("Process %d ended, marking stream %d as ended", pid, handler.metadata.streamID)
			_ = handler.markEnd()
			sc.telemetry.finalizedProcesses.Inc()
		}
	}
}
