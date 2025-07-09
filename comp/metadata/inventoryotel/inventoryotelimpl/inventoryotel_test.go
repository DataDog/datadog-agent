// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventoryotelimpl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func getProvides(t *testing.T, confOverrides map[string]any) (provides, error) {
	return newInventoryOtelProvider(
		fxutil.Test[dependencies](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			config.MockModule(),
			fx.Replace(config.MockParams{Overrides: confOverrides}),
			fx.Provide(func() serializer.MetricSerializer { return serializermock.NewMetricSerializer(t) }),
			fx.Provide(func() ipc.Component { return ipcmock.New(t) }),
			fx.Provide(func(ipcComp ipc.Component) ipc.HTTPClient { return ipcComp.GetClient() }),
			hostnameimpl.MockModule(),
		),
	)
}

func getTestInventoryPayload(t *testing.T, confOverrides map[string]any) *inventoryotel {
	p, _ := getProvides(t, confOverrides)
	return p.Comp.(*inventoryotel)
}

func TestGetPayload(t *testing.T) {
	overrides := map[string]any{
		"otelcollector.enabled":               true,
		"otelcollector.submit_dummy_metadata": true,
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

func TestFlareProviderFilename(t *testing.T) {
	io := getTestInventoryPayload(t, nil)
	assert.Equal(t, "otel.json", io.FlareFileName)
}

func TestConfigRefresh(t *testing.T) {
	cfg := configmock.New(t)
	io := getTestInventoryPayload(t, nil)

	assert.False(t, io.RefreshTriggered())
	cfg.Set("inventories_max_interval", 10*60, pkgconfigmodel.SourceAgentRuntime)
	assert.Eventually(t, func() bool {
		return assert.True(t, io.RefreshTriggered())
	}, 5*time.Second, 200*time.Millisecond)
}
