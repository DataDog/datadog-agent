// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package softwareinventoryimpl

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/inventory/software"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type testFixture struct {
	t                 *testing.T
	sysProbeClient    *mockSysProbeClient
	eventPlatformMock *mockEventPlatform
	reqs              Requires
}

type mockEventPlatform struct {
	mock.Mock
	mu sync.Mutex
}

func (m *mockEventPlatform) SendEventPlatformEvent(msg *message.Message, eventType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	args := m.Called(msg, eventType)
	return args.Error(0)
}

func (m *mockEventPlatform) SendEventPlatformEventBlocking(msg *message.Message, eventType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	args := m.Called(msg, eventType)
	return args.Error(0)
}

func (m *mockEventPlatform) Purge() map[string][]*message.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(map[string][]*message.Message)
}

func (m *mockEventPlatform) GetCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Calls)
}

func newFixtureWithData(t *testing.T, enabled bool, mockData []software.Entry) *testFixture {
	sp := &mockSysProbeClient{}
	sp.On("GetCheck", sysconfig.SoftwareInventoryModule).Return(mockData, nil)

	logComp := logmock.New(t)
	hostnameComp := &mockHostname{}

	configComp := config.NewMock(t)
	configComp.SetWithoutSource("software_inventory.enabled", enabled)

	// Create a mock event platform forwarder
	epMock := &mockEventPlatform{}
	epMock.On("SendEventPlatformEvent", mock.Anything, eventplatform.EventTypeSoftwareInventory).Return(nil)
	eventPlatformComp := option.NewPtr[eventplatform.Forwarder](epMock)

	return &testFixture{
		t:                 t,
		sysProbeClient:    sp,
		eventPlatformMock: epMock,
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

// softwareInventoryTestHelper wraps softwareInventory with test-specific wait helpers
type softwareInventoryTestHelper struct {
	*softwareInventory
	fixture *testFixture
}

// WaitForSystemProbe waits for the GetCheck method to be called on the sys probe client
func (h *softwareInventoryTestHelper) WaitForSystemProbe() *softwareInventoryTestHelper {
	require.Eventually(h.fixture.t, func() bool {
		return h.fixture.sysProbeClient.GetCallCount() > 0
	}, time.Second, 10*time.Millisecond, "Expected GetCheck to be called")
	return h
}

// WaitForPayload waits for a payload to be sent to the event platform forwarder
func (h *softwareInventoryTestHelper) WaitForPayload() *softwareInventoryTestHelper {
	require.Eventually(h.fixture.t, func() bool {
		return h.fixture.eventPlatformMock.GetCallCount() > 0
	}, time.Second, 10*time.Millisecond, "Expected payload to be sent")
	return h
}

// gets system under test
func (tf *testFixture) sut() *softwareInventoryTestHelper {
	// Pass a no-op sleep function for tests
	noopSleep := func(time.Duration) {
		// No-op: don't actually sleep in tests
	}

	provides, err := newWithClient(tf.reqs, tf.sysProbeClient, noopSleep)
	require.NoError(tf.t, err)

	is := provides.Comp.(*softwareInventory)

	helper := &softwareInventoryTestHelper{
		softwareInventory: is,
		fixture:           tf,
	}

	// Default wait for enabled components - gives goroutine time to start
	if is.enabled {
		time.Sleep(10 * time.Millisecond)
	}

	return helper
}

func TestFlareProviderOutputDisabled(t *testing.T) {
	f := newFixtureWithData(t, false, []software.Entry{{DisplayName: "TestApp"}})
	sut := f.sut()

	flareProvider := sut.FlareProvider()
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
	f.sysProbeClient.On("GetCheck", sysconfig.SoftwareInventoryModule).Return(nil, errors.New("error"))
	sut := f.sut().WaitForSystemProbe()

	flareProvider := sut.FlareProvider()
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
	sut := f.sut().WaitForPayload()

	flareProvider := sut.FlareProvider()
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
	sut := f.sut().WaitForPayload()

	w := httptest.NewRecorder()
	sut.writePayloadAsJSON(w, nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Contains(t, response, "host_software")
}

func TestGetPayload(t *testing.T) {
	f := newFixtureWithData(t, true, []software.Entry{{DisplayName: "TestApp", ProductCode: "test"}})
	sut := f.sut().WaitForPayload()

	payload := sut.getPayload()
	assert.NotNil(t, payload)
	p, ok := payload.(*Payload)
	assert.True(t, ok)
	assert.Len(t, p.Metadata.Software, 1)
	assert.Equal(t, "test", p.Metadata.Software[0].ProductCode)
	assert.Equal(t, "TestApp", p.Metadata.Software[0].DisplayName)

	// Test error case
	f = newFixtureWithData(t, true, []software.Entry{})
	sut = f.sut().WaitForPayload()

	payload = sut.getPayload()
	assert.NotNil(t, payload)
	p, ok = payload.(*Payload)
	assert.True(t, ok)
	assert.Len(t, p.Metadata.Software, 0)
}
