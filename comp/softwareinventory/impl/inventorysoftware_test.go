// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package softwareinventoryimpl

import (
	"encoding/json"
	"fmt"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/compression/selector"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/pkg/inventory/software"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/stretchr/testify/assert"
)

type testFixture struct {
	t              *testing.T
	sysProbeClient *mockSysProbeClient
	reqs           Requires
}

type mockCompressorComponent struct{}

func (*mockCompressorComponent) NewCompressor(_ string, _ int) compression.Compressor {
	return selector.NewNoopCompressor()
}

func newFixtureWithData(t *testing.T, enabled bool, mockData []software.Entry) *testFixture {
	sp := &mockSysProbeClient{}
	sp.On("GetCheck", sysconfig.SoftwareInventoryModule).Return(mockData, nil)

	logComp := logmock.New(t)
	hostnameComp := &mockHostname{}

	configComp := config.NewMock(t)
	configComp.SetWithoutSource("software_inventory.enabled", enabled)

	// Create a mock event platform component
	eventPlatformComp := option.NewPtr[eventplatform.Forwarder](eventplatformimpl.NewNoopEventPlatformForwarder(hostnameComp, &mockCompressorComponent{}))

	return &testFixture{
		t:              t,
		sysProbeClient: sp,
		reqs: Requires{
			Log:           logComp,
			Config:        configComp,
			Serializer:    serializermock.NewMetricSerializer(t),
			Hostname:      hostnameComp,
			Lc:            compdef.NewTestLifecycle(t),
			EventPlatform: eventPlatformComp,
		},
	}
}

// gets system under test
func (tf *testFixture) sut() *softwareInventory {
	provides, err := newWithClient(tf.reqs, tf.sysProbeClient)
	require.NoError(tf.t, err)

	return provides.Comp.(*softwareInventory)
}

func TestFlareProviderOutputDisabled(t *testing.T) {
	f := newFixtureWithData(t, false, []software.Entry{{DisplayName: "TestApp"}})
	is := f.sut()

	flareProvider := is.FlareProvider()
	assert.NotNil(t, flareProvider)
	assert.NotNil(t, flareProvider.FlareFiller)
	assert.NotNil(t, flareProvider.FlareFiller.Callback)

	// Create a mock FlareBuilder to test the callback
	mockBuilder := helpers.NewFlareBuilderMock(t, false)
	err := flareProvider.FlareFiller.Callback(mockBuilder)
	assert.NoError(t, err)

	// Verify that the file does not exist since the module is disabled.
	mockBuilder.AssertFileExists(flareFileName)
	mockBuilder.AssertFileContent("Software Inventory component is not enabled", flareFileName)
}

func TestFlareProviderOutputFailed(t *testing.T) {
	f := newFixtureWithData(t, true, []software.Entry{{DisplayName: "TestApp"}})
	f.sysProbeClient = &mockSysProbeClient{}
	f.sysProbeClient.On("GetCheck", sysconfig.SoftwareInventoryModule).Return(nil, fmt.Errorf("error"))
	is := f.sut()

	flareProvider := is.FlareProvider()
	assert.NotNil(t, flareProvider)
	assert.NotNil(t, flareProvider.FlareFiller)
	assert.NotNil(t, flareProvider.FlareFiller.Callback)

	// Create a mock FlareBuilder to test the callback
	mockBuilder := helpers.NewFlareBuilderMock(t, false)
	err := flareProvider.FlareFiller.Callback(mockBuilder)
	assert.NoError(t, err)

	// Verify that the file does not exist since the module is disabled.
	mockBuilder.AssertFileExists(flareFileName)
	mockBuilder.AssertFileContent("Software inventory data collection failed or returned no results", flareFileName)
}

func TestFlareProviderOutput(t *testing.T) {
	f := newFixtureWithData(t, true, []software.Entry{{DisplayName: "TestApp"}})
	is := f.sut()

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
	f := newFixtureWithData(t, true, []software.Entry{{DisplayName: "TestApp"}})
	is := f.sut()

	w := httptest.NewRecorder()
	is.writePayloadAsJSON(w, nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Contains(t, response, "host_software")
}

func TestGetPayload(t *testing.T) {
	f := newFixtureWithData(t, true, []software.Entry{{DisplayName: "TestApp", ProductCode: "test"}})
	is := f.sut()

	payload := is.getPayload()
	assert.NotNil(t, payload)
	p, ok := payload.(*Payload)
	assert.True(t, ok)
	assert.Len(t, p.Metadata.Software, 1)
	assert.Equal(t, "test", p.Metadata.Software[0].ProductCode)
	assert.Equal(t, "TestApp", p.Metadata.Software[0].DisplayName)

	// Test error case
	f = newFixtureWithData(t, true, []software.Entry{})
	is = f.sut()

	payload = is.getPayload()
	assert.NotNil(t, payload)
	p, ok = payload.(*Payload)
	assert.True(t, ok)
	assert.Len(t, p.Metadata.Software, 0)
}
