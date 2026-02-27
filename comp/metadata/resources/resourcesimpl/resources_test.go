// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package resourcesimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	testifyMock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestConfDisabled(t *testing.T) {
	overrides := map[string]any{
		"metadata_providers": []map[string]interface{}{
			{
				"name":     "resources",
				"interval": 0,
			},
		},
	}

	ret := newResourcesProvider(
		fxutil.Test[dependencies](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component { return config.NewMockWithOverrides(t, overrides) }),
			fx.Provide(func() serializer.MetricSerializer { return nil }),
			hostnameimpl.MockModule(),
		),
	)

	// When interval is 0 the resource Provider should be nil
	assert.Nil(t, ret.Provider.Callback)
}

func TestConfInterval(t *testing.T) {
	overrides := map[string]any{
		"metadata_providers": []map[string]interface{}{
			{
				"name":     "resources",
				"interval": 21,
			},
		},
	}

	ret := newResourcesProvider(
		fxutil.Test[dependencies](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component { return config.NewMockWithOverrides(t, overrides) }),
			fx.Provide(func() serializer.MetricSerializer { return nil }),
			hostnameimpl.MockModule(),
		),
	)

	assert.NotNil(t, ret.Provider.Callback)

	assert.Equal(t, 21*time.Second, ret.Comp.(*resourcesImpl).collectInterval)
}

func TestCollect(t *testing.T) {
	defer func(f func() (interface{}, error)) { collectResources = f }(collectResources)
	collectResources = func() (interface{}, error) {
		return []string{"proc1", "proc2", "proc3"}, nil
	}

	expectedPayload := "{\"resources\":{\"meta\":{\"host\":\"resources-test-hostname\"},\"processes\":{\"snaps\":[[\"proc1\",\"proc2\",\"proc3\"]]}}}"

	s := serializermock.NewMetricSerializer(t)
	s.On("SendProcessesMetadata",
		testifyMock.MatchedBy(func(payload map[string]interface{}) bool {
			jsonPayload, err := json.Marshal(payload)
			require.NoError(t, err)
			assert.Equal(t, expectedPayload, string(jsonPayload))
			return bytes.Equal(jsonPayload, []byte(expectedPayload))
		}),
	).Return(nil)

	ret := newResourcesProvider(
		fxutil.Test[dependencies](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component { return config.NewMock(t) }),
			fx.Provide(func() serializer.MetricSerializer { return s }),
			hostnameimpl.MockModule(),
		),
	)

	r := ret.Comp.(*resourcesImpl)
	r.hostname = "resources-test-hostname"

	interval := r.collect(context.Background())
	assert.Equal(t, defaultCollectInterval, interval)
	s.AssertExpectations(t)
}

func TestCollectError(t *testing.T) {
	defer func(f func() (interface{}, error)) { collectResources = f }(collectResources)
	collectResources = func() (interface{}, error) {
		return nil, errors.New("some error from gohai")
	}

	s := serializermock.NewMetricSerializer(t)
	ret := newResourcesProvider(
		fxutil.Test[dependencies](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component { return config.NewMock(t) }),
			fx.Provide(func() serializer.MetricSerializer { return s }),
			hostnameimpl.MockModule(),
		),
	)

	r := ret.Comp.(*resourcesImpl)
	interval := r.collect(context.Background())
	assert.Equal(t, defaultCollectInterval, interval)
	s.AssertExpectations(t)
}
