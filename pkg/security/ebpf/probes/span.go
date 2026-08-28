// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probes holds probes related files
package probes

import manager "github.com/DataDog/ebpf-manager"

// getSpanFillTailCallRoutes returns the tail call routes used to defer the Go
// pprof labels snapshot (collect_go_labels) and event emission to a single
// dedicated program, keeping the calling hooks within the verifier budget on
// old kernels.
func getSpanFillTailCallRoutes() []manager.TailCallRoute {
	return []manager.TailCallRoute{
		{
			ProgArrayName: "span_fill_progs",
			Key:           uint32(0),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tailCallFnc("fill_span_and_send"),
			},
		},
		// Tracepoint-typed twin, tail-called from handle_sys_*_exit (which run
		// in tracepoint context); bpf_tail_call requires a matching program type.
		{
			ProgArrayName: "span_fill_tp_progs",
			Key:           uint32(0),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tailCallTracepointFnc("fill_span_and_send"),
			},
		},
		// Index 1: setsockopt-specific program (reads the dedicated setsockopt_event
		// map, whose 4KB filter buffer can't use the shared span_fill slot).
		{
			ProgArrayName: "span_fill_progs",
			Key:           uint32(1),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tailCallFnc("fill_span_and_send_setsockopt"),
			},
		},
		{
			ProgArrayName: "span_fill_tp_progs",
			Key:           uint32(1),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tailCallTracepointFnc("fill_span_and_send_setsockopt"),
			},
		},
		// Index 2: exit-specific program (fill+send then unregister_go_labels).
		// Exit is reached only from kprobe/fentry (do_exit), so there is no
		// tracepoint-typed twin.
		{
			ProgArrayName: "span_fill_progs",
			Key:           uint32(2),
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: tailCallFnc("fill_span_and_send_exit"),
			},
		},
	}
}
