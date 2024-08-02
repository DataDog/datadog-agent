// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package tests

import (
	"testing"

	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/DataDog/datadog-agent/pkg/network/tracer"
	tracertestutil "github.com/DataDog/datadog-agent/pkg/network/tracer/testutil"
)

func platformInit() {
	_ = driver.Init(&sysconfigtypes.Config{})
}

func TestProtocolClassification(t *testing.T) {
	t.Run("without nat", func(t *testing.T) {
		testProtocolClassificationCrossOS(t, nil, "localhost", "127.0.0.1", "127.0.0.1")
	})
}

func testProtocolClassificationInner(t *testing.T, params protocolClassificationAttributes, _ *tracer.Tracer) {
	if params.skipCallback != nil {
		params.skipCallback(t, params.context)
	}

	if params.teardown != nil {
		t.Cleanup(func() {
			params.teardown(t, params.context)
		})
	}
	if params.preTracerSetup != nil {
		params.preTracerSetup(t, params.context)
	}
	cfg := tracertestutil.Config()
	tr := setupTracer(t, cfg)
	params.postTracerSetup(t, params.context)
	params.validation(t, params.context, tr)
}
