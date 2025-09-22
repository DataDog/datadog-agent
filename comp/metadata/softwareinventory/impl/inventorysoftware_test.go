// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package softwareinventoryimpl

import (
	"encoding/json"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/inventory/software"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/stretchr/testify/assert"
)

func newSoftwareInventory(t *testing.T, enabled bool, mockData []software.Entry) (*softwareInventory, *mockSysProbeClient) {
	sp := &mockSysProbeClient{}

	sp.On("GetCheck", sysconfig.SoftwareInventoryModule).Return(mockData, nil)

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
		Lc:         compdef.NewTestLifecycle(t),
	}

	// Call the constructor directly with the mock client
	provides, err := newWithClient(reqs, sp)
	require.NoError(t, err)

	return provides.Comp.(*softwareInventory), sp
}

func TestFlareProviderOutputDisabled(t *testing.T) {
	is, _ := newSoftwareInventory(t, false, []software.Entry{{DisplayName: "TestApp"}})

	flareProvider := is.FlareProvider()
	assert.NotNil(t, flareProvider)
	assert.NotNil(t, flareProvider.FlareFiller)
	assert.NotNil(t, flareProvider.FlareFiller.Callback)

	// Create a mock FlareBuilder to test the callback
	mockBuilder := helpers.NewFlareBuilderMock(t, false)
	err := flareProvider.FlareFiller.Callback(mockBuilder)
	assert.NoError(t, err)

	// Verify that the file does not exist since the module is disabled.
	mockBuilder.AssertNoFileExists(flareFileName)
}

func TestFlareProviderOutput(t *testing.T) {
	is, _ := newSoftwareInventory(t, true, []software.Entry{{DisplayName: "TestApp"}})

	flareProvider := is.FlareProvider()
	assert.NotNil(t, flareProvider)
	assert.NotNil(t, flareProvider.FlareFiller)
	assert.NotNil(t, flareProvider.FlareFiller.Callback)

	// Create a mock FlareBuilder to test the callback
	mockBuilder := helpers.NewFlareBuilderMock(t, false)
	err := flareProvider.FlareFiller.Callback(mockBuilder)
	assert.NoError(t, err)

	// Verify the mock builder was called with the expected file
	mockBuilder.AssertFileExists(flareFileName)
}

func TestWritePayloadAsJSON(t *testing.T) {
	is, _ := newSoftwareInventory(t, true, []software.Entry{{DisplayName: "TestApp"}})

	w := httptest.NewRecorder()
	is.writePayloadAsJSON(w, nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Contains(t, response, "host_software")
}

func TestGetPayload(t *testing.T) {
	is, _ := newSoftwareInventory(t, true, []software.Entry{{DisplayName: "TestApp", ProductCode: "test"}})

	payload := is.getPayload()
	assert.NotNil(t, payload)
	p, ok := payload.(*Payload)
	assert.True(t, ok)
	assert.Len(t, p.Metadata.Software, 1)
	assert.Equal(t, "test", p.Metadata.Software[0].ProductCode)
	assert.Equal(t, "TestApp", p.Metadata.Software[0].DisplayName)

	// Test error case
	is, _ = newSoftwareInventory(t, true, []software.Entry{})

	payload = is.getPayload()
	assert.NotNil(t, payload)
	p, ok = payload.(*Payload)
	assert.True(t, ok)
	assert.Len(t, p.Metadata.Software, 0)
}
