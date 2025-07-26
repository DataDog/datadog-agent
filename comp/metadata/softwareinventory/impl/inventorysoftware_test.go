// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package softwareinventoryimpl

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"path"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/inventory/software"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/stretchr/testify/assert"
)

func newSoftwareInventory(t *testing.T, enabled bool) (*softwareInventory, *mockSysProbeClient) {
	sp := &mockSysProbeClient{}

	// Create dependencies manually for the test
	logComp := logmock.New(t)
	hostnameComp := &mockHostname{}

	configComp := config.NewMock(t)
	configComp.SetWithoutSource("software_inventory.enabled", enabled)

	// Create the Requires struct manually
	reqs := Requires{
		Log:        logComp,
		Config:     configComp,
		Serializer: serializermock.NewMetricSerializer(t),
		Hostname:   hostnameComp,
	}

	// Call the constructor directly with the mock client
	provides, err := newWithClient(reqs, sp)
	require.NoError(t, err)

	return provides.Comp.(*softwareInventory), sp
}

func TestRefreshCachedValues(t *testing.T) {
	mockData := []software.Entry{{DisplayName: "FooApp", ProductCode: "foo"}, {DisplayName: "BarApp", ProductCode: "bar"}}
	is, sp := newSoftwareInventory(t, true)
	sp.On("GetCheck", sysconfig.SoftwareInventory).Return(mockData, nil)

	err := is.refreshCachedValues()

	// Assert that the cached values were refreshed
	assert.NoError(t, err)
	assert.Len(t, is.cachedInventory, 2)
	assert.Equal(t, "foo", is.cachedInventory[0].ProductCode)
	assert.Equal(t, "FooApp", is.cachedInventory[0].DisplayName)
	assert.Equal(t, "bar", is.cachedInventory[1].ProductCode)
	assert.Equal(t, "BarApp", is.cachedInventory[1].DisplayName)
	sp.AssertNumberOfCalls(t, "GetCheck", 1)
}

func TestRefreshCachedValuesWithError(t *testing.T) {
	is, sp := newSoftwareInventory(t, true)
	sp.On("GetCheck", sysconfig.SoftwareInventory).Return([]software.Entry{}, fmt.Errorf("error"))

	// Assert that we attempted to refresh the cached values but
	// system probe returned an error
	assert.Error(t, is.refreshCachedValues())
	assert.Len(t, is.cachedInventory, 0)
	sp.AssertNumberOfCalls(t, "GetCheck", 1)
}

func TestRefreshCachedValuesWithEmptyInventory(t *testing.T) {
	is, sp := newSoftwareInventory(t, true)
	sp.On("GetCheck", sysconfig.SoftwareInventory).Return([]software.Entry{}, nil)

	err := is.refreshCachedValues()
	assert.NoError(t, err)
	assert.Empty(t, is.cachedInventory)
}

func TestFlareProviderOutputDisabled(t *testing.T) {
	mockData := []software.Entry{{DisplayName: "TestApp"}}
	is, sp := newSoftwareInventory(t, false)
	sp.On("GetCheck", sysconfig.SoftwareInventory).Return(mockData, nil)

	flareProvider := is.FlareProvider()
	assert.NotNil(t, flareProvider)
	assert.NotNil(t, flareProvider.FlareFiller)
	assert.NotNil(t, flareProvider.FlareFiller.Callback)

	// Create a mock FlareBuilder to test the callback
	mockBuilder := helpers.NewFlareBuilderMock(t, false)
	err := flareProvider.FlareFiller.Callback(mockBuilder)
	assert.NoError(t, err)

	// Verify that the file does not exists since the module is disabled.
	mockBuilder.AssertNoFileExists(path.Join("metadata", "inventory", flareFileName))
}

func TestFlareProviderOutput(t *testing.T) {
	mockData := []software.Entry{{DisplayName: "TestApp"}}
	is, sp := newSoftwareInventory(t, true)
	sp.On("GetCheck", sysconfig.SoftwareInventory).Return(mockData, nil)

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
	mockData := []software.Entry{{DisplayName: "TestApp"}}
	is, sp := newSoftwareInventory(t, true)
	sp.On("GetCheck", sysconfig.SoftwareInventory).Return(mockData, nil)

	w := httptest.NewRecorder()
	is.writePayloadAsJSON(w, nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Contains(t, response, "host_software")
}

func TestGetPayload(t *testing.T) {
	mockData := []software.Entry{{DisplayName: "TestApp", ProductCode: "test"}}
	is, sp := newSoftwareInventory(t, true)
	sp.On("GetCheck", sysconfig.SoftwareInventory).Return(mockData, nil)

	payload := is.getPayload()
	assert.NotNil(t, payload)
	p, ok := payload.(*Payload)
	assert.True(t, ok)
	assert.Len(t, p.Metadata.Software, 1)
	assert.Equal(t, "test", p.Metadata.Software[0].ProductCode)
	assert.Equal(t, "TestApp", p.Metadata.Software[0].DisplayName)

	// Test error case
	is, sp = newSoftwareInventory(t, true)
	sp.On("GetCheck", sysconfig.SoftwareInventory).Return([]software.Entry{}, fmt.Errorf("error"))

	payload = is.getPayload()
	assert.Nil(t, payload)
}

func TestComponentRefresh(t *testing.T) {
	is, _ := newSoftwareInventory(t, true)
	// Refresh should not panic
	assert.NotPanics(t, func() {
		is.Refresh()
	})
}
