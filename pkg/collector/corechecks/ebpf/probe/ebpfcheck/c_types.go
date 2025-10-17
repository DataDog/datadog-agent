// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore

package ebpfcheck

/*
#include "../../c/runtime/ebpf-kern-user.h"
*/
import "C"

const ioctlCollectKprobeMissedStatsCmd = C.EBPF_CHECK_KPROBE_MISSES_CMD

type perfBufferKey C.perf_buffer_key_t
type mmapRegion C.mmap_region_t
type ringMmap C.ring_mmap_t
type cookie C.cookie_t
type kStatsError C.k_stats_error_t
type kprobeKernelStats C.kprobe_stats_t
type kprobeStatsCollectorErrors C.stats_collector_error_t
type kprobeStatsErrors C.k_stats_error_t

func (err *kprobeStatsErrors) String() string {
	switch err.Type {
	case 1:
		return "fd is not a perf event fd"
	case 2:
		return "perf event fd does not have a kprobe"
	case 3:
		return "unable to get perf_event from file"
	case 4:
		return "could not read the pmu type of the perf event"
	case 5:
		return "could not read kprobe hits"
	case 6:
		return "could not read kprobe misses"
	case 7:
		return "could not read kretprobe misses"
	case 8:
		return "could not read struct trace_event_call flags"
	case 9:
		return "could not check if perf event is associated with a tracefs kprobe"
	case 10:
		return "could not read trace_kprobe from perf_event"
	default:
		return "unknown error in bpf program"
	}
}
