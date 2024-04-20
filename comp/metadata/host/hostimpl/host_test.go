// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostimpl

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
	"golang.org/x/exp/maps"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flarehelpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/resources"
	"github.com/DataDog/datadog-agent/comp/metadata/resources/resourcesimpl"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestNewHostProviderDefaultInterval(t *testing.T) {
	ret := newHostProvider(
		fxutil.Test[dependencies](
			t,
			logimpl.MockModule(),
			config.MockModule(),
			resourcesimpl.MockModule(),
			fx.Replace(resources.MockParams{Data: nil}),
			fx.Provide(func() serializer.MetricSerializer { return nil }),
		),
	)

	assert.Equal(t, defaultCollectInterval, ret.Comp.(*host).collectInterval)
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
			logimpl.MockModule(),
			config.MockModule(),
			resourcesimpl.MockModule(),
			fx.Replace(resources.MockParams{Data: nil}),
			fx.Replace(config.MockParams{Overrides: overrides}),
			fx.Provide(func() serializer.MetricSerializer { return nil }),
		),
	)

	assert.Equal(t, time.Duration(1000)*time.Second, ret.Comp.(*host).collectInterval)
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
			logimpl.MockModule(),
			config.MockModule(),
			resourcesimpl.MockModule(),
			fx.Replace(resources.MockParams{Data: nil}),
			fx.Replace(config.MockParams{Overrides: overrides}),
			fx.Provide(func() serializer.MetricSerializer { return nil }),
		),
	)

	assert.Equal(t, defaultCollectInterval, ret.Comp.(*host).collectInterval)
}

func TestFlareProvider(t *testing.T) {
	ret := newHostProvider(
		fxutil.Test[dependencies](
			t,
			logimpl.MockModule(),
			config.MockModule(),
			resourcesimpl.MockModule(),
			fx.Replace(resources.MockParams{Data: nil}),
			fx.Provide(func() serializer.MetricSerializer { return nil }),
		),
	)

	hostProvider := ret.Comp.(*host)
	fbMock := flarehelpers.NewFlareBuilderMock(t, false)
	hostProvider.fillFlare(fbMock.Fb)

	fbMock.AssertFileExists(filepath.Join("metadata", "host.json"))
}

func TestStatusHeaderProvider(t *testing.T) {
	ret := newHostProvider(
		fxutil.Test[dependencies](
			t,
			logimpl.MockModule(),
			config.MockModule(),
			resourcesimpl.MockModule(),
			fx.Replace(resources.MockParams{Data: nil}),
			fx.Provide(func() serializer.MetricSerializer { return nil }),
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
