// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventoryotelimpl

import (
	"bytes"
	"testing"
	"time"

	"go.uber.org/fx"
	"golang.org/x/exp/maps"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authtokenimpl "github.com/DataDog/datadog-agent/comp/api/authtoken/fetchonlyimpl"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func getProvides(t *testing.T, confOverrides map[string]any) provides {
	return newInventoryOtelProvider(
		fxutil.Test[dependencies](
			t,
			logimpl.MockModule(),
			config.MockModule(),
			fx.Replace(config.MockParams{Overrides: confOverrides}),
			fx.Provide(func() serializer.MetricSerializer { return &serializer.MockSerializer{} }),
			authtokenimpl.Module(),
		),
	)
}

func getTestInventoryPayload(t *testing.T, confOverrides map[string]any) *inventoryotel {
	p := getProvides(t, confOverrides)
	return p.Comp.(*inventoryotel)
}

func TestGetPayload(t *testing.T) {
	overrides := map[string]any{
		"otel.enabled":                           true,
		"otel.submit_dummy_inventories_metadata": true,
	}

	io := getTestInventoryPayload(t, overrides)
	io.hostname = "hostname-for-test"

	startTime := time.Now().UnixNano()

	p := io.getPayload()
	payload := p.(*Payload)

	// payload should contain dummy data
	d, err := io.fetchDummyOtelConfig(nil)
	require.Nil(t, err)

	data := copyAndScrub(d)

	assert.True(t, payload.Timestamp > startTime)
	assert.Equal(t, "hostname-for-test", payload.Hostname)
	assert.Equal(t, data, payload.Metadata)
}

func TestGet(t *testing.T) {
	overrides := map[string]any{
		"otel.enabled":                           true,
		"otel.submit_dummy_inventories_metadata": true,
	}
	io := getTestInventoryPayload(t, overrides)

	// Collect metadata
	io.refreshMetadata()

	p := io.Get()

	// Grab dummy data
	d, err := io.fetchDummyOtelConfig(nil)
	assert.Nil(t, err)
	assert.Equal(t, d, io.data)

	// verify that the return map is a copy
	p["otel_customer_configuration"] = ""
	assert.NotEqual(t, p["otel_customer_configuration"], io.data["otel_customer_configuration"])
}

func TestFlareProviderFilename(t *testing.T) {
	io := getTestInventoryPayload(t, nil)
	assert.Equal(t, "otel.json", io.FlareFileName)
}

func TestConfigRefresh(t *testing.T) {
	io := getTestInventoryPayload(t, nil)

	assert.False(t, io.RefreshTriggered())
	pkgconfig.Datadog.Set("inventories_max_interval", 10*60, pkgconfigmodel.SourceAgentRuntime)
	assert.True(t, io.RefreshTriggered())
}

func TestStatusHeaderProvider(t *testing.T) {
	ret := getProvides(t, nil)

	headerStatusProvider := ret.StatusHeaderProvider.Provider

	tests := []struct {
		name       string
		assertFunc func(t *testing.T)
	}{
		{"JSON", func(t *testing.T) {
			stats := make(map[string]interface{})
			headerStatusProvider.JSON(false, stats)

			keys := maps.Keys(stats)

			assert.Contains(t, keys, "otel_metadata")
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
