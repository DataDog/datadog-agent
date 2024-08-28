// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package tests

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/tracer"
)

func platformInit() {
	// linux-specific tasks here
}

func testProtocolClassificationInner(t *testing.T, params protocolClassificationAttributes, tr *tracer.Tracer) {
	if params.skipCallback != nil {
		params.skipCallback(t, params.context)
	}
	t.Cleanup(func() { tr.RemoveClient(clientID) })
	t.Cleanup(func() { tr.Pause() })

	if params.teardown != nil {
		t.Cleanup(func() {
			params.teardown(t, params.context)
		})
	}

	require.NoError(t, tr.Pause(), "disable probes - before pre tracer")
	if params.preTracerSetup != nil {
		params.preTracerSetup(t, params.context)
	}

	tr.RemoveClient(clientID)
	require.NoError(t, tr.RegisterClient(clientID))
	require.NoError(t, tr.Resume(), "enable probes - before post tracer")
	if params.postTracerSetup != nil {
		params.postTracerSetup(t, params.context)
	}
	require.NoError(t, tr.Pause(), "disable probes - after post tracer")

	params.validation(t, params.context, tr)
}
