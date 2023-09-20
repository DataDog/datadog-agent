// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package resources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	testifyMock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestConfDisabled(t *testing.T) {
	overrides := map[string]any{
		"metadata_providers": []configUtils.MetadataProviders{
			{
				Name:     "resources",
				Interval: 0,
			},
		},
	}

	ret := newResourcesProvider(
		fxutil.Test[dependencies](
			t,
			log.MockModule,
			config.MockModule,
			fx.Replace(config.MockParams{Overrides: overrides}),
			fx.Provide(func() serializer.MetricSerializer { return nil }),
		),
	)

	// When interval is 0 the resource Provider should be an empty Optional[T]
	assert.False(t, ret.Provider.Callback.IsSet())
}

func TestConfInterval(t *testing.T) {
	overrides := map[string]any{
		"metadata_providers": []configUtils.MetadataProviders{
			{
				Name:     "resources",
				Interval: 21,
			},
		},
	}

	ret := newResourcesProvider(
		fxutil.Test[dependencies](
			t,
			log.MockModule,
			config.MockModule,
			fx.Replace(config.MockParams{Overrides: overrides}),
			fx.Provide(func() serializer.MetricSerializer { return nil }),
		),
	)

	assert.Equal(t, 21*time.Second, ret.Comp.(resources).collectInterval)
}

func TestCollect(t *testing.T) {
	defer func(f func() (interface{}, error)) { collectResources = f }(collectResources)
	collectResources = func() (interface{}, error) {
		return []string{"proc1", "proc2", "proc3"}, nil
	}

	expectedPayload := "{\"resources\":{\"meta\":{\"host\":\"resources-test-hostname\"},\"processes\":{\"snaps\":[[\"proc1\",\"proc2\",\"proc3\"]]}}}"

	s := &serializer.MockSerializer{}
	s.On("SendProcessesMetadata",
		testifyMock.MatchedBy(func(payload map[string]interface{}) bool {
			jsonPayload, err := json.Marshal(payload)
			require.NoError(t, err)
			assert.Equal(t, expectedPayload, string(jsonPayload))
			return bytes.Compare(jsonPayload, []byte(expectedPayload)) == 0
		}),
	).Return(nil)

	ret := newResourcesProvider(
		fxutil.Test[dependencies](
			t,
			log.MockModule,
			config.MockModule,
			fx.Provide(func() serializer.MetricSerializer { return s }),
		),
	)

	r := ret.Comp.(resources)
	r.hostname = "resources-test-hostname"

	interval := r.collect(context.Background())
	assert.Equal(t, defaultCollectInterval, interval)
	s.AssertExpectations(t)
}

func TestCollectError(t *testing.T) {
	defer func(f func() (interface{}, error)) { collectResources = f }(collectResources)
	collectResources = func() (interface{}, error) {
		return nil, fmt.Errorf("some error from gohai")
	}

	s := &serializer.MockSerializer{}
	ret := newResourcesProvider(
		fxutil.Test[dependencies](
			t,
			log.MockModule,
			config.MockModule,
			fx.Provide(func() serializer.MetricSerializer { return s }),
		),
	)

	r := ret.Comp.(resources)
	interval := r.collect(context.Background())
	assert.Equal(t, defaultCollectInterval, interval)
	s.AssertExpectations(t)
}
