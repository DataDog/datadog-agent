// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package infra

import (
	"bytes"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/auto/events"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const eksClusterURN = "urn:pulumi:e2e-stack::my-project::aws:eks:Cluster::myeks"

func TestFormatEngineEvent(t *testing.T) {
	stepMetadata := func(op apitype.OpType, urn, resType string) apitype.StepEventMetadata {
		return apitype.StepEventMetadata{Op: op, URN: urn, Type: resType}
	}

	tests := []struct {
		name string
		in   events.EngineEvent
		out  string
		show bool
	}{
		{
			name: "resource-pre-event-create",
			in:   events.EngineEvent{EngineEvent: apitype.EngineEvent{ResourcePreEvent: &apitype.ResourcePreEvent{Metadata: stepMetadata(apitype.OpCreate, eksClusterURN, "aws:eks:Cluster")}}},
			out:  "  ⏳ aws:eks:Cluster myeks creating...",
			show: true,
		},
		{
			name: "res-outputs-event-create",
			in:   events.EngineEvent{EngineEvent: apitype.EngineEvent{ResOutputsEvent: &apitype.ResOutputsEvent{Metadata: stepMetadata(apitype.OpCreate, eksClusterURN, "aws:eks:Cluster")}}},
			out:  "  ✓ aws:eks:Cluster myeks created",
			show: true,
		},
		{
			name: "resource-pre-event-update",
			in:   events.EngineEvent{EngineEvent: apitype.EngineEvent{ResourcePreEvent: &apitype.ResourcePreEvent{Metadata: stepMetadata(apitype.OpUpdate, "urn:pulumi:e2e-stack::my-project::aws:ec2/instance:Instance::aws-vm", "aws:ec2/instance:Instance")}}},
			out:  "  ⏳ aws:ec2/instance:Instance aws-vm updating...",
			show: true,
		},
		{
			name: "res-outputs-event-delete",
			in:   events.EngineEvent{EngineEvent: apitype.EngineEvent{ResOutputsEvent: &apitype.ResOutputsEvent{Metadata: stepMetadata(apitype.OpDelete, "urn:pulumi:e2e-stack::my-project::aws:s3:Bucket::my-bucket", "aws:s3:Bucket")}}},
			out:  "  ✓ aws:s3:Bucket my-bucket deleted",
			show: true,
		},
		{
			name: "res-outputs-event-create-replacement",
			in:   events.EngineEvent{EngineEvent: apitype.EngineEvent{ResOutputsEvent: &apitype.ResOutputsEvent{Metadata: stepMetadata(apitype.OpCreateReplacement, "urn:pulumi:e2e-stack::my-project::aws:ec2:Instance::aws-vm", "aws:ec2:Instance")}}},
			out:  "  ✓ aws:ec2:Instance aws-vm created replacement",
			show: true,
		},
		{
			name: "res-op-failed-event",
			in:   events.EngineEvent{EngineEvent: apitype.EngineEvent{ResOpFailedEvent: &apitype.ResOpFailedEvent{Metadata: stepMetadata(apitype.OpCreate, "urn:pulumi:e2e-stack::my-project::aws:iam:Role::myeks-role", "aws:iam:Role")}}},
			out:  "  ✗ aws:iam:Role myeks-role create failed",
			show: true,
		},
		{
			name: "provider-resource-is-internal",
			in:   events.EngineEvent{EngineEvent: apitype.EngineEvent{ResourcePreEvent: &apitype.ResourcePreEvent{Metadata: stepMetadata(apitype.OpRead, "urn:pulumi:e2e-stack::my-project::pulumi:providers:aws::default_6_0_0", "pulumi:providers:aws")}}},
			show: false,
		},
		{
			name: "step-without-urn-is-internal",
			in:   events.EngineEvent{EngineEvent: apitype.EngineEvent{ResourcePreEvent: &apitype.ResourcePreEvent{Metadata: stepMetadata(apitype.OpCreate, "", "aws:eks:Cluster")}}},
			show: false,
		},
		{
			name: "diagnostic-error",
			in:   events.EngineEvent{EngineEvent: apitype.EngineEvent{DiagnosticEvent: &apitype.DiagnosticEvent{Severity: "error", Message: "update failed"}}},
			out:  "✗ error: update failed",
			show: true,
		},
		{
			name: "diagnostic-warning",
			in:   events.EngineEvent{EngineEvent: apitype.EngineEvent{DiagnosticEvent: &apitype.DiagnosticEvent{Severity: "warning", Message: "deprecated field"}}},
			out:  "⚠ warning: deprecated field",
			show: true,
		},
		{
			name: "diagnostic-info-with-urn",
			in:   events.EngineEvent{EngineEvent: apitype.EngineEvent{DiagnosticEvent: &apitype.DiagnosticEvent{Severity: "info", URN: eksClusterURN, Message: "cluster will restart"}}},
			out:  "ℹ cluster will restart",
			show: true,
		},
		{
			name: "diagnostic-info-without-urn-is-noise",
			in:   events.EngineEvent{EngineEvent: apitype.EngineEvent{DiagnosticEvent: &apitype.DiagnosticEvent{Severity: "info", Message: "loading provider"}}},
			show: false,
		},
		{
			name: "summary-event",
			in: events.EngineEvent{EngineEvent: apitype.EngineEvent{SummaryEvent: &apitype.SummaryEvent{
				ResourceChanges: map[apitype.OpType]int{apitype.OpCreate: 18, apitype.OpUpdate: 1},
				DurationSeconds: 131,
				Result:          apitype.OperationResultSucceeded,
			}}},
			out:  "📊 18 created, 1 updated (2m11s)",
			show: true,
		},
		{
			name: "summary-event-no-changes",
			in:   events.EngineEvent{EngineEvent: apitype.EngineEvent{SummaryEvent: &apitype.SummaryEvent{ResourceChanges: map[apitype.OpType]int{}, DurationSeconds: 5}}},
			out:  "📊 no changes (5s)",
			show: true,
		},
		{
			name: "summary-event-failed-result",
			in: events.EngineEvent{EngineEvent: apitype.EngineEvent{SummaryEvent: &apitype.SummaryEvent{
				ResourceChanges: map[apitype.OpType]int{apitype.OpSame: 3, apitype.OpCreate: 2},
				DurationSeconds: 45,
				Result:          apitype.OperationResultFailed,
			}}},
			out:  "📊 3 unchanged, 2 created (45s) — failed",
			show: true,
		},
		{
			name: "cancel-event-is-completion-signal",
			in:   events.EngineEvent{EngineEvent: apitype.EngineEvent{CancelEvent: &apitype.CancelEvent{}}},
			show: false, // SDK emits on success too; not a cancellation
		},
		{
			name: "progress-event-is-noise",
			in:   events.EngineEvent{EngineEvent: apitype.EngineEvent{ProgressEvent: &apitype.ProgressEvent{Type: apitype.PluginDownload, Message: "downloading provider"}}},
			show: false,
		},
		{
			name: "stdout-event-is-noise",
			in:   events.EngineEvent{EngineEvent: apitype.EngineEvent{StdoutEvent: &apitype.StdoutEngineEvent{Message: "loading policy pack"}}},
			show: false,
		},
		{
			name: "prelude-event-is-noise",
			in:   events.EngineEvent{EngineEvent: apitype.EngineEvent{PreludeEvent: &apitype.PreludeEvent{}}},
			show: false,
		},
		{
			name: "stream-error",
			in:   events.EngineEvent{Error: errors.New("unexpected end of JSON input")},
			out:  "⚠ event stream error: unexpected end of JSON input",
			show: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, show := formatEngineEvent(tc.in)
			assert.Equal(t, tc.show, show)
			if tc.show {
				assert.Equal(t, tc.out, out)
			}
		})
	}
}

// syncedBuffer is an io.Writer safe for concurrent reads and writes, so the
// output of the event stream consumer goroutine can be polled while it runs.
type syncedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestStartEventStreamLogger(t *testing.T) {
	logger := &syncedBuffer{}
	ch := startEventStreamLogger(logger)

	eventsToSend := []events.EngineEvent{
		{EngineEvent: apitype.EngineEvent{ResourcePreEvent: &apitype.ResourcePreEvent{
			Metadata: apitype.StepEventMetadata{Op: apitype.OpCreate, URN: eksClusterURN, Type: "aws:eks:Cluster"},
		}}},
		{EngineEvent: apitype.EngineEvent{ResOutputsEvent: &apitype.ResOutputsEvent{
			Metadata: apitype.StepEventMetadata{Op: apitype.OpCreate, URN: eksClusterURN, Type: "aws:eks:Cluster"},
		}}},
		{EngineEvent: apitype.EngineEvent{ProgressEvent: &apitype.ProgressEvent{Message: "downloading provider"}}},
		{EngineEvent: apitype.EngineEvent{SummaryEvent: &apitype.SummaryEvent{
			ResourceChanges: map[apitype.OpType]int{apitype.OpCreate: 1},
			DurationSeconds: 10,
			Result:          apitype.OperationResultSucceeded,
		}}},
	}

	for _, event := range eventsToSend {
		ch <- event
	}
	// The Pulumi SDK closes the channel once all events are sent; the consumer
	// goroutine then drains the buffer and stops.
	close(ch)

	expected := "" +
		"  ⏳ aws:eks:Cluster myeks creating...\n" +
		"  ✓ aws:eks:Cluster myeks created\n" +
		"📊 1 created (10s)\n"
	require.Eventually(t, func() bool {
		return logger.String() == expected
	}, 5*time.Second, 10*time.Millisecond)
}

func TestFormatDurationSeconds(t *testing.T) {
	assert.Equal(t, "45s", formatDurationSeconds(45))
	assert.Equal(t, "2m11s", formatDurationSeconds(131))
	assert.Equal(t, "1m0s", formatDurationSeconds(60))
}
