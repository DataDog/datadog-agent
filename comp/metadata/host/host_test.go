// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package host

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/metadata/resources"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestNewHostProviderDefaultInterval(t *testing.T) {
	ret := newHostProvider(
		fxutil.Test[dependencies](
			t,
			log.MockModule,
			config.MockModule,
			resources.MockModule,
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
			log.MockModule,
			config.MockModule,
			resources.MockModule,
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
			log.MockModule,
			config.MockModule,
			resources.MockModule,
			fx.Replace(resources.MockParams{Data: nil}),
			fx.Replace(config.MockParams{Overrides: overrides}),
			fx.Provide(func() serializer.MetricSerializer { return nil }),
		),
	)

	assert.Equal(t, defaultCollectInterval, ret.Comp.(*host).collectInterval)
}
