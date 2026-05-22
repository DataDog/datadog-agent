// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostimpl

import (
	"bytes"
	"context"
	"maps"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flarehelpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/metadata/resources/def"
	resourcesmock "github.com/DataDog/datadog-agent/comp/metadata/resources/mock"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// testDeps is an fx-compatible bridge used in tests, since Requires uses compdef.In (not fx.In).
type testDeps struct {
	fx.In

	Log        log.Component
	Config     config.Component
	Resources  resources.Component
	Serializer serializer.MetricSerializer
	Hostname   hostnameinterface.Component
}

func makeRequires(deps testDeps) Requires {
	return Requires{
		Log:        deps.Log,
		Config:     deps.Config,
		Resources:  deps.Resources,
		Serializer: deps.Serializer,
		Hostname:   deps.Hostname,
	}
}

func TestNewHostProviderDefaultIntervals(t *testing.T) {
	ret := NewComponent(
		makeRequires(fxutil.Test[testDeps](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component { return config.NewMock(t) }),
			resourcesmock.MockModule(),
			fx.Replace(resourcesmock.MockParams{Data: nil}),
			fx.Provide(func() serializer.MetricSerializer { return nil }),
			hostnameimpl.MockModule(),
		)),
	)

	assert.Equal(t, defaultCollectInterval, ret.Comp.(*host).backoffPolicy.MaxInterval)
	assert.Equal(t, defaultEarlyInterval, ret.Comp.(*host).backoffPolicy.InitialInterval)
}

func TestNewHostProviderIntervalValidation(t *testing.T) {
	tests := []struct {
		name                 string
		mainInterval         time.Duration
		earlyInterval        time.Duration
		expectedMaxInterval  time.Duration
		expectedInitInterval time.Duration
	}{
		{
			name:                 "both intervals valid",
			mainInterval:         1800,
			earlyInterval:        600,
			expectedMaxInterval:  1800 * time.Second,
			expectedInitInterval: 600 * time.Second,
		},
		{
			name:                 "both intervals invalid - too low",
			mainInterval:         30,
			earlyInterval:        30,
			expectedMaxInterval:  defaultCollectInterval,
			expectedInitInterval: defaultEarlyInterval, // main invalid means whole provider ignored
		},
		{
			name:                 "main valid, early invalid - too low",
			mainInterval:         1800,
			earlyInterval:        30,
			expectedMaxInterval:  1800 * time.Second,
			expectedInitInterval: defaultEarlyInterval,
		},
		{
			name:                 "main valid, early invalid - too high",
			mainInterval:         1800,
			earlyInterval:        15000,
			expectedMaxInterval:  1800 * time.Second,
			expectedInitInterval: defaultEarlyInterval,
		},
		{
			name:                 "main valid, early invalid - greater than main",
			mainInterval:         1800,
			earlyInterval:        2000,
			expectedMaxInterval:  1800 * time.Second,
			expectedInitInterval: defaultEarlyInterval,
		},
		{
			name:                 "main invalid, early valid",
			mainInterval:         30,
			earlyInterval:        600,
			expectedMaxInterval:  defaultCollectInterval,
			expectedInitInterval: defaultEarlyInterval, // main invalid means whole provider ignored
		},
		{
			name:                 "early interval zero uses default",
			mainInterval:         1800,
			earlyInterval:        0, // zero means use default
			expectedMaxInterval:  1800 * time.Second,
			expectedInitInterval: defaultEarlyInterval,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overrides := map[string]any{
				"metadata_providers": []map[string]interface{}{
					{
						"name":           "host",
						"interval":       tt.mainInterval,
						"early_interval": tt.earlyInterval,
					},
				},
			}

			ret := NewComponent(
				makeRequires(fxutil.Test[testDeps](
					t,
					fx.Provide(func() log.Component { return logmock.New(t) }),
					fx.Provide(func() config.Component { return config.NewMockWithOverrides(t, overrides) }),
					resourcesmock.MockModule(),
					fx.Replace(resourcesmock.MockParams{Data: nil}),
					fx.Provide(func() serializer.MetricSerializer { return nil }),
					hostnameimpl.MockModule(),
				)),
			)

			hostProvider := ret.Comp.(*host)
			assert.Equal(t, tt.expectedMaxInterval, hostProvider.backoffPolicy.MaxInterval, tt.name)
			assert.Equal(t, tt.expectedInitInterval, hostProvider.backoffPolicy.InitialInterval, tt.name)
			assert.Equal(t, 3.0, hostProvider.backoffPolicy.Multiplier, tt.name)
			assert.Equal(t, 0.0, hostProvider.backoffPolicy.RandomizationFactor, tt.name)
		})
	}
}

func TestBackoffWhenEarlyIntervalEqualsCollectionInterval(t *testing.T) {
	overrides := map[string]any{
		"metadata_providers": []map[string]interface{}{
			{
				"name": "host", "interval": 300, "early_interval": 300,
			},
		},
	}
	ret := NewComponent(makeRequires(fxutil.Test[testDeps](t,
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component { return config.NewMockWithOverrides(t, overrides) }),
		resourcesmock.MockModule(),
		fx.Replace(resourcesmock.MockParams{Data: nil}),
		fx.Provide(func() serializer.MetricSerializer { return nil }),
		hostnameimpl.MockModule(),
	)))
	h := ret.Comp.(*host)

	h.backoffPolicy.Reset()
	for i := 0; i < 5; i++ {
		assert.InDelta(t, 300*time.Second, h.backoffPolicy.NextBackOff(), 1)
	}
}

func TestFlareProvider(t *testing.T) {
	ret := NewComponent(
		makeRequires(fxutil.Test[testDeps](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component { return config.NewMock(t) }),
			resourcesmock.MockModule(),
			fx.Replace(resourcesmock.MockParams{Data: nil}),
			fx.Provide(func() serializer.MetricSerializer { return nil }),
			hostnameimpl.MockModule(),
		)),
	)

	hostProvider := ret.Comp.(*host)
	fbMock := flarehelpers.NewFlareBuilderMock(t, false)
	hostProvider.fillFlare(context.Background(), fbMock)

	fbMock.AssertFileExists(filepath.Join("metadata", "host.json"))
}

func TestStatusHeaderProvider(t *testing.T) {
	ret := NewComponent(
		makeRequires(fxutil.Test[testDeps](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component { return config.NewMock(t) }),
			resourcesmock.MockModule(),
			fx.Replace(resourcesmock.MockParams{Data: nil}),
			fx.Provide(func() serializer.MetricSerializer { return nil }),
			hostnameimpl.MockModule(),
		)),
	)

	headerStatusProvider := ret.StatusHeaderProvider.Provider

	tests := []struct {
		name       string
		assertFunc func(t *testing.T)
	}{
		{"JSON", func(t *testing.T) {
			stats := make(map[string]interface{})
			headerStatusProvider.JSON(false, stats)

			keys := slices.Collect(maps.Keys(stats))

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
