// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventorysoftware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
)

func getProvides(t *testing.T, confOverrides map[string]any) (Provides, *mockSysProbeClient) {
	sp := &mockSysProbeClient{}
	provides := NewWithClient(
		fxutil.Test[Dependencies](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			config.MockModule(),
			fx.Replace(config.MockParams{Overrides: confOverrides}),
			fx.Provide(func() serializer.MetricSerializer { return serializermock.NewMetricSerializer(t) }),
		),
		sp,
	)
	return provides, sp
}

func newInventorySoftware(t *testing.T, confOverrides map[string]any) (*inventorySoftware, *mockSysProbeClient) {
	p, c := getProvides(t, confOverrides)
	return p.Comp.(*inventorySoftware), c
}

func TestRefreshCachedValues(t *testing.T) {
	mockData := SoftwareInventoryMap{
		"foo": {"DisplayName": "FooApp"},
		"bar": {"DisplayName": "BarApp"},
	}
	is, sp := newInventorySoftware(t, nil)
	sp.On("GetCheck", sysconfig.InventorySoftwareModule).Return(mockData, nil)

	err := is.refreshCachedValues()

	// Assert that the cached values were refreshed
	assert.NoError(t, err)
	assert.Len(t, is.cachedInventory, 2)
	assert.Equal(t, "foo", is.cachedInventory[0].ProductCode)
	assert.Equal(t, "FooApp", is.cachedInventory[0].Metadata["DisplayName"])
	assert.Equal(t, "bar", is.cachedInventory[1].ProductCode)
	assert.Equal(t, "BarApp", is.cachedInventory[1].Metadata["DisplayName"])
	sp.AssertNumberOfCalls(t, "GetCheck", 1)
}

func TestRefreshCachedValuesWithError(t *testing.T) {
	is, sp := newInventorySoftware(t, nil)
	sp.On("GetCheck", sysconfig.InventorySoftwareModule).Return(SoftwareInventoryMap{}, fmt.Errorf("error"))

	// Assert that we attempted to refresh the cached values but
	// system probe returned an error
	assert.Error(t, is.refreshCachedValues())
	assert.Len(t, is.cachedInventory, 0)
	sp.AssertNumberOfCalls(t, "GetCheck", 1)
}

func TestRefreshCachedValuesWithEmptyInventory(t *testing.T) {
	is, sp := newInventorySoftware(t, nil)
	sp.On("GetCheck", sysconfig.InventorySoftwareModule).Return(SoftwareInventoryMap{}, nil)

	err := is.refreshCachedValues()
	assert.NoError(t, err)
	assert.Empty(t, is.cachedInventory)
}

func TestFlareProviderOutput(t *testing.T) {
	mockData := SoftwareInventoryMap{
		"test": {"DisplayName": "TestApp"},
	}
	is, sp := newInventorySoftware(t, nil)
	sp.On("GetCheck", sysconfig.InventorySoftwareModule).Return(mockData, nil)

	flareProvider := is.FlareProvider()
	assert.NotNil(t, flareProvider)
	assert.NotNil(t, flareProvider.FlareFiller)
	assert.NotNil(t, flareProvider.FlareFiller.Callback)

	// Create a mock FlareBuilder to test the callback
	mockBuilder := helpers.NewFlareBuilderMock(t, false)
	err := flareProvider.FlareFiller.Callback(mockBuilder)
	assert.NoError(t, err)

	// Verify the mock builder was called with the expected file
	mockBuilder.AssertFileExists(path.Join("metadata", "inventory", flareFileName))
}

func TestWritePayloadAsJSON(t *testing.T) {
	mockData := SoftwareInventoryMap{
		"test": {"DisplayName": "TestApp"},
	}
	is, sp := newInventorySoftware(t, nil)
	sp.On("GetCheck", sysconfig.InventorySoftwareModule).Return(mockData, nil)

	w := httptest.NewRecorder()
	is.writePayloadAsJSON(w, nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Contains(t, response, "software_inventory_metadata")
}

func TestGetPayload(t *testing.T) {
	mockData := SoftwareInventoryMap{
		"test": {"DisplayName": "TestApp"},
	}
	is, sp := newInventorySoftware(t, nil)
	sp.On("GetCheck", sysconfig.InventorySoftwareModule).Return(mockData, nil)

	payload := is.getPayload()
	assert.NotNil(t, payload)
	p, ok := payload.(*Payload)
	assert.True(t, ok)
	assert.Len(t, p.Metadata, 1)
	assert.Equal(t, "test", p.Metadata[0].ProductCode)
	assert.Equal(t, "TestApp", p.Metadata[0].Metadata["DisplayName"])

	// Test error case
	is, sp = newInventorySoftware(t, nil)
	sp.On("GetCheck", sysconfig.InventorySoftwareModule).Return(SoftwareInventoryMap{}, fmt.Errorf("error"))

	payload = is.getPayload()
	assert.Nil(t, payload)
}

func TestComponentRefresh(t *testing.T) {
	is, _ := newInventorySoftware(t, nil)
	// Refresh should not panic
	assert.NotPanics(t, func() {
		is.Refresh()
	})
}
