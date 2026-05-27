// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package ddhostnameprocessor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pprofile"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// resourceAccessor abstracts signal-specific resource access so test logic is written once.
type resourceAccessor struct {
	appendEmpty func() pcommon.Map
	at          func(i int) pcommon.Map
	process     func(p *ddhostnameProcessor) error
}

type signal struct {
	name        string
	newAccessor func() resourceAccessor
}

var allSignals = []signal{
	{"profiles", func() resourceAccessor {
		pd := pprofile.NewProfiles()
		return resourceAccessor{
			appendEmpty: func() pcommon.Map { return pd.ResourceProfiles().AppendEmpty().Resource().Attributes() },
			at:          func(i int) pcommon.Map { return pd.ResourceProfiles().At(i).Resource().Attributes() },
			process: func(p *ddhostnameProcessor) error {
				_, err := p.processProfiles(context.Background(), pd)
				return err
			},
		}
	}},
	{"metrics", func() resourceAccessor {
		md := pmetric.NewMetrics()
		return resourceAccessor{
			appendEmpty: func() pcommon.Map { return md.ResourceMetrics().AppendEmpty().Resource().Attributes() },
			at:          func(i int) pcommon.Map { return md.ResourceMetrics().At(i).Resource().Attributes() },
			process: func(p *ddhostnameProcessor) error {
				_, err := p.processMetrics(context.Background(), md)
				return err
			},
		}
	}},
	{"logs", func() resourceAccessor {
		ld := plog.NewLogs()
		return resourceAccessor{
			appendEmpty: func() pcommon.Map { return ld.ResourceLogs().AppendEmpty().Resource().Attributes() },
			at:          func(i int) pcommon.Map { return ld.ResourceLogs().At(i).Resource().Attributes() },
			process: func(p *ddhostnameProcessor) error {
				_, err := p.processLogs(context.Background(), ld)
				return err
			},
		}
	}},
	{"traces", func() resourceAccessor {
		td := ptrace.NewTraces()
		return resourceAccessor{
			appendEmpty: func() pcommon.Map { return td.ResourceSpans().AppendEmpty().Resource().Attributes() },
			at:          func(i int) pcommon.Map { return td.ResourceSpans().At(i).Resource().Attributes() },
			process: func(p *ddhostnameProcessor) error {
				_, err := p.processTraces(context.Background(), td)
				return err
			},
		}
	}},
}

// TestMultipleResources_MixedPriority verifies injection, priority, and passthrough
// in a single batch across all signal types:
//   - Resource 0: no host -> injected
//   - Resource 1: user-set datadog.host.name -> preserved (priority)
//   - Resource 2: other attrs, no host -> injected
func TestMultipleResources_MixedPriority(t *testing.T) {
	for _, sig := range allSignals {
		t.Run(sig.name, func(t *testing.T) {
			ra := sig.newAccessor()
			ra.appendEmpty().FromRaw(map[string]any{})
			ra.appendEmpty().FromRaw(map[string]any{"datadog.host.name": "user-host"})
			ra.appendEmpty().FromRaw(map[string]any{"service.name": "svc"})

			p := &ddhostnameProcessor{host: "resolved-host"}
			require.NoError(t, ra.process(p))

			assert.Equal(t, "resolved-host", ra.at(0).AsRaw()["datadog.host.name"], "empty resource should get resolved host")
			assert.Equal(t, "user-host", ra.at(1).AsRaw()["datadog.host.name"], "user-set host must have priority")
			assert.Equal(t, "resolved-host", ra.at(2).AsRaw()["datadog.host.name"], "resource without host should get resolved host")
			assert.Equal(t, "svc", ra.at(2).AsRaw()["service.name"], "other attributes should be preserved")
		})
	}
}
