// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package tracer

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func testProtocolClassificationInner(t *testing.T, params protocolClassificationAttributes, tr *Tracer) {
	if params.skipCallback != nil {
		params.skipCallback(t, params.context)
	}
	t.Cleanup(func() { tr.removeClient(clientID) })
	t.Cleanup(func() { tr.ebpfTracer.Pause() })

	if params.teardown != nil {
		t.Cleanup(func() {
			params.teardown(t, params.context)
		})
	}

	require.NoError(t, tr.ebpfTracer.Pause(), "disable probes - before pre tracer")
	if params.preTracerSetup != nil {
		params.preTracerSetup(t, params.context)
	}

	var kStatsPre map[string]int64
	if m, _ := tr.getStats(kprobesStats); m != nil {
		if m := m["kprobes"]; m != nil {
			kStatsPre = m.(map[string]int64)
		}
	}

	t.Cleanup(func() {
		if kStatsPre != nil && t.Failed() {
			var kStatsPost map[string]int64
			if m, _ := tr.getStats(kprobesStats); m != nil {
				if m := m["kprobes"]; m != nil {
					kStatsPost = m.(map[string]int64)
				}
				if kStatsPost == nil {
					return
				}
				for k, pre := range kStatsPre {
					if !strings.HasSuffix(k, "_misses") {
						continue
					}
					if post, ok := kStatsPost[k]; ok && pre != post {
						t.Logf("kprobe stat %s differs pre=%v post=%v", k, pre, post)
					}
				}
			}
		}
	})

	tr.removeClient(clientID)
	initTracerState(t, tr)
	require.NoError(t, tr.ebpfTracer.Resume(), "enable probes - before post tracer")
	params.postTracerSetup(t, params.context)
	require.NoError(t, tr.ebpfTracer.Pause(), "disable probes - after post tracer")

	params.validation(t, params.context, tr)
}
