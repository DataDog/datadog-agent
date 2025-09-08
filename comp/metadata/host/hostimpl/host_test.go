// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostimpl

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/fx"
	"golang.org/x/exp/maps"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flarehelpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/metadata/resources"
	"github.com/DataDog/datadog-agent/comp/metadata/resources/resourcesimpl"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestNewHostProviderDefaultInterval(t *testing.T) {
	ret := newHostProvider(
		fxutil.Test[dependencies](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component { return config.NewMock(t) }),
			resourcesimpl.MockModule(),
			fx.Replace(resources.MockParams{Data: nil}),
			fx.Provide(func() serializer.MetricSerializer { return nil }),
			hostnameimpl.MockModule(),
		),
	)

	assert.Equal(t, defaultCollectInterval, ret.Comp.(*host).backoffPolicy.MaxInterval)
}

func TestNewHostProviderCustomInterval(t *testing.T) {
	overrides := map[string]any{
		"metadata_providers": []configUtils.MetadataProviders{
			{
				Name:     "host",
				Interval: 1000,
			},
		},
	}

	ret := newHostProvider(
		fxutil.Test[dependencies](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component { return config.NewMockWithOverrides(t, overrides) }),
			resourcesimpl.MockModule(),
			fx.Replace(resources.MockParams{Data: nil}),
			fx.Provide(func() serializer.MetricSerializer { return nil }),
			hostnameimpl.MockModule(),
		),
	)

	assert.Equal(t, time.Duration(1000)*time.Second, ret.Comp.(*host).backoffPolicy.MaxInterval)
}

func TestNewHostProviderInvalidCustomInterval(t *testing.T) {
	overrides := map[string]any{
		"metadata_providers": []configUtils.MetadataProviders{
			{
				Name:     "host",
				Interval: 100, // interval too low, should be ignored
			},
		},
	}

	ret := newHostProvider(
		fxutil.Test[dependencies](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component { return config.NewMockWithOverrides(t, overrides) }),
			resourcesimpl.MockModule(),
			fx.Replace(resources.MockParams{Data: nil}),
			fx.Provide(func() serializer.MetricSerializer { return nil }),
			hostnameimpl.MockModule(),
		),
	)

	assert.Equal(t, defaultCollectInterval, ret.Comp.(*host).backoffPolicy.MaxInterval)
}

func TestFlareProvider(t *testing.T) {
	ret := newHostProvider(
		fxutil.Test[dependencies](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component { return config.NewMock(t) }),
			resourcesimpl.MockModule(),
			fx.Replace(resources.MockParams{Data: nil}),
			fx.Provide(func() serializer.MetricSerializer { return nil }),
			hostnameimpl.MockModule(),
		),
	)

	hostProvider := ret.Comp.(*host)
	fbMock := flarehelpers.NewFlareBuilderMock(t, false)
	hostProvider.fillFlare(fbMock)

	fbMock.AssertFileExists(filepath.Join("metadata", "host.json"))
}

func TestStatusHeaderProvider(t *testing.T) {
	ret := newHostProvider(
		fxutil.Test[dependencies](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component { return config.NewMock(t) }),
			resourcesimpl.MockModule(),
			fx.Replace(resources.MockParams{Data: nil}),
			fx.Provide(func() serializer.MetricSerializer { return nil }),
			hostnameimpl.MockModule(),
		),
	)

	headerStatusProvider := ret.StatusHeaderProvider.Provider

	tests := []struct {
		name       string
		assertFunc func(t *testing.T)
	}{
		{"JSON", func(t *testing.T) {
			stats := make(map[string]interface{})
			headerStatusProvider.JSON(false, stats)

			keys := maps.Keys(stats)

			assert.Contains(t, keys, "hostnameStats")
			assert.Contains(t, keys, "hostTags")
			assert.Contains(t, keys, "hostinfo")
			assert.Contains(t, keys, "metadata")
		}},
		{"Text", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := headerStatusProvider.Text(false, b)

			assert.NoError(t, err)

			assert.NotEmpty(t, b.String())
		}},
		{"HTML", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := headerStatusProvider.HTML(false, b)

			assert.NoError(t, err)

			assert.NotEmpty(t, b.String())
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.assertFunc(t)
		})
	}
}

func TestExponentialBackoffIntervals(t *testing.T) {
	mockSerializer := serializermock.NewMetricSerializer(t)
	mockSerializer.On("SendHostMetadata", mock.Anything).Return(nil)

	ret := newHostProvider(
		fxutil.Test[dependencies](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component { return config.NewMock(t) }),
			resourcesimpl.MockModule(),
			fx.Replace(resources.MockParams{Data: nil}),
			fx.Provide(func() serializer.MetricSerializer { return mockSerializer }),
			hostnameimpl.MockModule(),
		),
	)

	hostProvider := ret.Comp.(*host)

	hostProvider.backoffPolicy = &backoff.ExponentialBackOff{
		InitialInterval:     5 * time.Minute,
		RandomizationFactor: 0.0,
		Multiplier:          3.0,
		MaxInterval:         defaultCollectInterval,
		MaxElapsedTime:      0,
		Clock:               backoff.SystemClock,
	}
	hostProvider.backoffPolicy.Reset()

	ctx := context.Background()

	expectedIntervals := []time.Duration{
		5 * time.Minute,
		15 * time.Minute,
		defaultCollectInterval,
		defaultCollectInterval,
	}

	for i, expected := range expectedIntervals {
		actual := hostProvider.collect(ctx)
		assert.Equal(t, expected, actual, "Interval %d should be exactly %v, got %v", i, expected, actual)
	}

	for i := 0; i < 3; i++ {
		actual := hostProvider.collect(ctx)
		assert.Equal(t, defaultCollectInterval, actual, "Stabilized interval %d should be exactly %v, got %v", i, defaultCollectInterval, actual)
	}
}

func TestExponentialBackoffInitialization(t *testing.T) {
	ret := newHostProvider(
		fxutil.Test[dependencies](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component { return config.NewMock(t) }),
			resourcesimpl.MockModule(),
			fx.Replace(resources.MockParams{Data: nil}),
			fx.Provide(func() serializer.MetricSerializer { return serializermock.NewMetricSerializer(t) }),
			hostnameimpl.MockModule(),
		),
	)

	hostProvider := ret.Comp.(*host)

	assert.NotNil(t, hostProvider.backoffPolicy, "Backoff policy should be initialized")
	assert.Equal(t, 5*time.Minute, hostProvider.backoffPolicy.InitialInterval, "Initial interval should be 5 minutes")
	assert.Equal(t, defaultCollectInterval, hostProvider.backoffPolicy.MaxInterval, "Max interval should match configured interval")
	assert.Equal(t, 3.0, hostProvider.backoffPolicy.Multiplier, "Multiplier should be 3.0")
	assert.Equal(t, 0.0, hostProvider.backoffPolicy.RandomizationFactor, "Randomization factor should be 0.0")
}
