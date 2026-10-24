// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helper

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func TestTelemetryProviderServerForwardsFilteredMetrics(t *testing.T) {
	tele := telemetrymock.New(t)

	forwarded := tele.NewGaugeWithOpts("helper_test", "forwarded", nil, "metric that must be forwarded", coretelemetry.DefaultOptions)
	dropped := tele.NewGaugeWithOpts("helper_test", "dropped", nil, "metric that must be filtered out", coretelemetry.DefaultOptions)
	forwarded.Set(1)
	dropped.Set(2)

	server := NewTelemetryProviderServer(tele, coretelemetry.StaticMetricFilter("helper_test__forwarded"))

	resp, err := server.GetTelemetry(context.Background(), &pbcore.GetTelemetryRequest{})
	require.NoError(t, err)

	promText, ok := resp.Payload.(*pbcore.GetTelemetryResponse_PromText)
	require.True(t, ok, "expected prometheus text payload")
	assert.Contains(t, promText.PromText, "helper_test__forwarded")
	assert.NotContains(t, promText.PromText, "helper_test__dropped")
}
