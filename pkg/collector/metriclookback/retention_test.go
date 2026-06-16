// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metriclookback

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/metriclookback/ringbuffer"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
)

type retainingSerializer struct {
	series []*metrics.Serie
	serializermock.MetricSerializer
}

func (s *retainingSerializer) SendIterableSeries(seriesSource metrics.SerieSource) error {
	for seriesSource.MoveNext() {
		s.series = append(s.series, seriesSource.Current())
	}
	return nil
}

func TestRetentionDumpSendsRetainedSamples(t *testing.T) {
	retention := NewRetention("host-a", ringbuffer.Options{})
	serializer := &retainingSerializer{}

	manager := retention.NewSenderManager(context.Background())
	sender, err := manager.GetSender(checkid.ID("dump-check"))
	require.NoError(t, err)
	sender.Gauge("dump.gauge", 1, "", nil)
	sender.Gauge("dump.other", 2, "", nil)
	sender.Commit()

	count, err := retention.Dump(serializer)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
	require.Len(t, serializer.series, 2)

	byName := map[string]float64{}
	for _, serie := range serializer.series {
		require.Len(t, serie.Points, 1)
		byName[serie.Name] = serie.Points[0].Value
	}
	assert.Equal(t, 1.0, byName["dump.gauge"])
	assert.Equal(t, 2.0, byName["dump.other"])

	// Dump is non-destructive: a second dump resends the same samples.
	count2, err := retention.Dump(serializer)
	require.NoError(t, err)
	assert.Equal(t, 2, count2)
}

func TestRetentionDumpEmptyBufferSendsNothing(t *testing.T) {
	retention := NewRetention("host-a", ringbuffer.Options{})
	serializer := &retainingSerializer{}

	count, err := retention.Dump(serializer)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
	assert.Empty(t, serializer.series)
}

func TestNewRetentionFromConfigHonorsEnabledFlag(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("metric_lookback.enabled", false)
	assert.Nil(t, NewRetentionFromConfig(cfg, "host-a"))

	cfg.SetInTest("metric_lookback.enabled", true)
	cfg.SetInTest("metric_lookback.capacity", 10)
	cfg.SetInTest("metric_lookback.shard_count", 2)
	assert.NotNil(t, NewRetentionFromConfig(cfg, "host-a"))
}

func TestRetentionDumpDisabledReturnsError(t *testing.T) {
	_, err := (*Retention)(nil).Dump(&retainingSerializer{})
	require.Error(t, err)
}

func TestRetentionDumpSerializerUnavailableReturnsError(t *testing.T) {
	retention := NewRetention("host-a", ringbuffer.Options{})
	_, err := retention.Dump(nil)
	require.Error(t, err)
}
