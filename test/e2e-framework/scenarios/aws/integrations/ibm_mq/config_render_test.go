// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ibm_mq

import (
	"strings"
	"testing"
)

func baseParams() *Params {
	return &Params{
		NQmgrs:                1,
		BasePort:              1414,
		Channel:               "DEV.ADMIN.SVRCONN",
		QMPrefix:              "QM",
		CollectResetQueue:     true,
		MinCollectionInterval: 15,
	}
}

func TestRenderQueueSelectionMutuallyExclusive(t *testing.T) {
	cases := []struct {
		name         string
		mutate       func(*Params)
		wantAuto     bool
		wantRegex    bool
		wantExplicit bool
	}{
		{"auto only", func(p *Params) { p.AutoDiscoverQueues = true }, true, false, false},
		{"regex wins over auto", func(p *Params) { p.AutoDiscoverQueues = true; p.QueueRegex = "DEV.*" }, false, true, false},
		{"explicit wins over auto", func(p *Params) { p.AutoDiscoverQueues = true; p.ExplicitQueues = []string{"DEV.QUEUE.1"} }, false, false, true},
		{"none set", func(p *Params) {}, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := baseParams()
			tc.mutate(p)
			out, err := renderCheckConfig(p)
			if err != nil {
				t.Fatalf("render error: %v", err)
			}
			if got := strings.Contains(out, "auto_discover_queues: true"); got != tc.wantAuto {
				t.Errorf("auto_discover_queues=%v want %v\n%s", got, tc.wantAuto, out)
			}
			if got := strings.Contains(out, "queue_regex:"); got != tc.wantRegex {
				t.Errorf("queue_regex=%v want %v\n%s", got, tc.wantRegex, out)
			}
			if got := strings.Contains(out, "\n    queues:"); got != tc.wantExplicit {
				t.Errorf("queues=%v want %v\n%s", got, tc.wantExplicit, out)
			}
		})
	}
}
