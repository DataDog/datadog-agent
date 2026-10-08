// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
)

// Payload handles the JSON unmarshalling of the metadata payload
type testPayload struct{}

func (p *testPayload) MarshalJSON() ([]byte, error) {
	return []byte("{\"test\": true}"), nil
}

func getTestInventoryPayload(t *testing.T, confOverrides map[string]any) *InventoryPayload {
	i := CreateInventoryPayload(
		config.NewMockWithOverrides(t, confOverrides),
		logmock.New(t),
		serializermock.NewMetricSerializer(t),
		func() marshaler.JSONMarshaler { return &testPayload{} },
		"test.json",
	)
	return &i
}

func getEmptyInventoryPayload(t *testing.T, confOverrides map[string]any) *InventoryPayload {
	i := CreateInventoryPayload(
		config.NewMockWithOverrides(t, confOverrides),
		logmock.New(t),
		serializermock.NewMetricSerializer(t),
		func() marshaler.JSONMarshaler { return nil },
		"testempty.json",
	)
	return &i
}

func TestEnabled(t *testing.T) {
	i := getTestInventoryPayload(t, map[string]any{
		"inventories_enabled": true,
	})

	assert.True(t, i.Enabled)
}

func TestDisabled(t *testing.T) {
	i := getTestInventoryPayload(t, map[string]any{
		"inventories_enabled": false,
	})

	assert.False(t, i.Enabled)
}

func TestDefaultInterval(t *testing.T) {
	i := getTestInventoryPayload(t, nil)

	assert.Equal(t, defaultMinInterval, i.MinInterval)
	assert.Equal(t, defaultMaxInterval, i.MaxInterval)
}

func TestInterval(t *testing.T) {
	i := getTestInventoryPayload(t, map[string]any{
		"inventories_min_interval": 123,
		"inventories_max_interval": 456,
	})

	assert.Equal(t, 123*time.Second, i.MinInterval)
	assert.Equal(t, 456*time.Second, i.MaxInterval)
}

func TestMetadataProvider(t *testing.T) {
	i := getTestInventoryPayload(t, nil)

	i.Enabled = true
	assert.NotNil(t, i.MetadataProvider().Callback)

	i.Enabled = false
	assert.Nil(t, i.MetadataProvider().Callback)
}

func TestFlareProvider(t *testing.T) {
	i := getTestInventoryPayload(t, nil)

	assert.NotNil(t, i.FlareProvider().FlareFiller.Callback)
}

func TestGetAsJSON(t *testing.T) {
	i := getTestInventoryPayload(t, nil)

	i.Enabled = false
	_, err := i.GetAsJSON()
	assert.Error(t, err)
}

func TestFillFlare(t *testing.T) {
	f := helpers.NewFlareBuilderMock(t, false)
	i := getTestInventoryPayload(t, nil)
	flareFiller := i.FlareProvider().FlareFiller.Callback

	i.Enabled = false
	flareFiller(context.Background(), f)
	f.AssertFileExists("metadata", "inventory", "test.json")
	f.AssertFileContent("inventory metadata is disabled", "metadata", "inventory", "test.json")

	i.Enabled = true
	flareFiller(context.Background(), f)
	f.AssertFileExists("metadata", "inventory", "test.json")
	f.AssertFileContent("{\n  \"test\": true\n}", "metadata", "inventory", "test.json")
}

func TestCollectRecentLastCollect(t *testing.T) {
	i := getTestInventoryPayload(t, nil)
	i.LastCollect = time.Now()
	i.createdAt = time.Now().Add(-2 * time.Minute)

	interval := i.collect(context.Background())
	assert.Equal(t, defaultMinInterval, interval)
	// check that no Payload was send since LastCollect is recent
	i.serializer.(*serializermock.MetricSerializer).AssertExpectations(t)
}

func TestCollectStartupTime(t *testing.T) {
	i := getTestInventoryPayload(t, nil)

	// testing collect do not send metadata if hasn't elapsed a minute from createdAt time
	createdAt := time.Now().Add(2 * time.Minute)
	i.createdAt = createdAt

	serializerMock := i.serializer.(*serializermock.MetricSerializer)
	duration := 1 * time.Minute

	interval := i.collect(context.Background())
	assert.Equal(t, duration-time.Since(createdAt).Round(duration), interval.Round(duration))
	assert.Empty(t, serializerMock.Calls)

	// testing with custom values from configuration
	i = getTestInventoryPayload(t, map[string]any{
		"inventories_first_run_delay": 0,
	})

	// reset serializer mock
	serializerMock = serializermock.NewMetricSerializer(t)

	serializerMock.On(
		"SendMetadata",
		mock.MatchedBy(func(m marshaler.JSONMarshaler) bool {
			if _, ok := m.(*testPayload); !ok {
				return false
			}
			return true
		})).Return(nil)

	i.serializer = serializerMock
	interval = i.collect(context.Background())
	assert.Equal(t, defaultMinInterval, interval)
	serializerMock.AssertExpectations(t)
}

func TestCollect(t *testing.T) {
	i := getTestInventoryPayload(t, nil)

	// Ensure calls to collect do not fail the check for createdAt
	i.createdAt = time.Now().Add(-2 * time.Minute)

	serializerMock := i.serializer.(*serializermock.MetricSerializer)

	// testing collect with LastCollect > MaxInterval

	serializerMock.On(
		"SendMetadata",
		mock.MatchedBy(func(m marshaler.JSONMarshaler) bool {
			if _, ok := m.(*testPayload); !ok {
				return false
			}
			return true
		})).Return(nil)

	// Make sure the minInterval between two payload has expired
	i.LastCollect = time.Now().Add(-1 * time.Hour)

	now := time.Now()
	interval := i.collect(context.Background())
	assert.Equal(t, defaultMinInterval, interval)
	assert.False(t, i.LastCollect.Before(now))
	i.serializer.(*serializermock.MetricSerializer).AssertExpectations(t)

	// testing collect with LastCollect between MinInterval and MaxInterval

	// reset serializer mock and test that a new call to Collect doesn't trigger a new payload
	serializerMock = serializermock.NewMetricSerializer(t)
	i.serializer = serializerMock

	i.LastCollect = time.Now().Add(-i.MinInterval + 1*time.Second)
	i.collect(context.Background())
	i.serializer.(*serializermock.MetricSerializer).AssertExpectations(t)

	// testing collect with LastCollect between MinInterval and MaxInterval with forceRefresh being trigger

	i.Refresh()
	assert.True(t, i.forceRefresh.Load())

	serializerMock.On(
		"SendMetadata",
		mock.MatchedBy(func(m marshaler.JSONMarshaler) bool {
			if _, ok := m.(*testPayload); !ok {
				return false
			}
			return true
		})).Return(nil)

	i.collect(context.Background())
	i.serializer.(*serializermock.MetricSerializer).AssertExpectations(t)
	assert.False(t, i.forceRefresh.Load())
}

func TestSubmitBypassesCollectionDelays(t *testing.T) {
	for _, tt := range []struct {
		name          string
		firstRunDelay int
	}{
		{name: "startup delay", firstRunDelay: 3600},
		{name: "recent collection"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			i := getTestInventoryPayload(t, map[string]any{
				"inventories_enabled":         true,
				"inventories_first_run_delay": tt.firstRunDelay,
			})
			i.LastCollect = time.Now()
			serializerMock := i.serializer.(*serializermock.MetricSerializer)
			i.collect(context.Background())
			serializerMock.AssertNotCalled(t, "SendMetadata", mock.Anything)

			serializerMock.On("SendMetadata", &testPayload{}).Return(nil).Twice()
			for range 2 {
				i.Refresh()
				before := time.Now()
				i.Submit()
				assert.False(t, i.LastCollect.Before(before))
				assert.False(t, i.LastCollect.After(time.Now()))
				assert.False(t, i.RefreshTriggered())
			}
			serializerMock.AssertNumberOfCalls(t, "SendMetadata", 2)
			i.collect(context.Background())
			serializerMock.AssertNumberOfCalls(t, "SendMetadata", 2)
		})
	}
}

func TestSubmitSkippedWithoutCollectionOrSerializer(t *testing.T) {
	for _, tt := range []struct {
		name       string
		enabled    bool
		serializer bool
	}{
		{name: "disabled", serializer: true},
		{name: "missing serializer", enabled: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			i := getTestInventoryPayload(t, map[string]any{"inventories_enabled": tt.enabled})
			i.getPayload = func() marshaler.JSONMarshaler {
				t.Fatal("skipped submission must not build a payload")
				return nil
			}
			if !tt.serializer {
				i.serializer = nil
			}
			lastCollect := time.Now().Add(-time.Hour)
			i.LastCollect = lastCollect
			i.forceRefresh.Store(true)
			i.SetReady(false)
			i.SetReady(true)
			i.Submit()
			assert.Equal(t, lastCollect, i.LastCollect)
			assert.True(t, i.forceRefresh.Load())
			if !tt.enabled {
				assert.Nil(t, i.MetadataProvider().Callback)
				i.serializer.(*serializermock.MetricSerializer).AssertNotCalled(t, "SendMetadata", mock.Anything)
			}
		})
	}
}

func TestSubmitRefreshAndScheduledCollection(t *testing.T) {
	i := getTestInventoryPayload(t, map[string]any{"inventories_first_run_delay": 0})
	serializerMock := i.serializer.(*serializermock.MetricSerializer)
	serializerMock.On("SendMetadata", &testPayload{}).Return(nil).Times(3)

	i.Refresh()
	i.Submit()
	assert.False(t, i.RefreshTriggered())
	assert.Equal(t, i.MinInterval, i.collect(context.Background()))
	serializerMock.AssertNumberOfCalls(t, "SendMetadata", 1)

	i.Refresh()
	assert.Equal(t, i.MinInterval, i.collect(context.Background()))
	assert.False(t, i.RefreshTriggered())
	serializerMock.AssertNumberOfCalls(t, "SendMetadata", 2)

	i.LastCollect = time.Now().Add(-i.MaxInterval)
	assert.Equal(t, i.MinInterval, i.collect(context.Background()))
	serializerMock.AssertNumberOfCalls(t, "SendMetadata", 3)
}

func TestSubmitPreservesRefreshTriggeredWhileBuildingPayload(t *testing.T) {
	i := getTestInventoryPayload(t, map[string]any{"inventories_first_run_delay": 0})
	serializerMock := i.serializer.(*serializermock.MetricSerializer)
	serializerMock.On("SendMetadata", &testPayload{}).Return(nil).Twice()
	i.getPayload = func() marshaler.JSONMarshaler {
		i.Refresh()
		return &testPayload{}
	}
	i.Submit()
	assert.True(t, i.RefreshTriggered())

	i.getPayload = func() marshaler.JSONMarshaler { return &testPayload{} }
	i.collect(context.Background())
	assert.False(t, i.RefreshTriggered())
	serializerMock.AssertNumberOfCalls(t, "SendMetadata", 2)
}

func TestSubmitEmptyPayloadAndSerializerError(t *testing.T) {
	for _, tt := range []struct {
		name         string
		emptyPayload bool
		sendError    error
	}{
		{name: "empty payload", emptyPayload: true},
		{name: "serializer error", sendError: errors.New("submission failed")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			i := getTestInventoryPayload(t, map[string]any{"inventories_first_run_delay": 0})
			serializerMock := i.serializer.(*serializermock.MetricSerializer)
			if tt.emptyPayload {
				i.getPayload = func() marshaler.JSONMarshaler { return nil }
			} else {
				serializerMock.On("SendMetadata", &testPayload{}).Return(tt.sendError).Once()
			}
			i.Refresh()
			before := time.Now()
			i.Submit()
			assert.False(t, i.LastCollect.Before(before))
			assert.False(t, i.RefreshTriggered())
			lastCollect := i.LastCollect
			i.collect(context.Background())
			assert.Equal(t, lastCollect, i.LastCollect)
			if tt.emptyPayload {
				serializerMock.AssertNotCalled(t, "SendMetadata", mock.Anything)
			} else {
				serializerMock.AssertNumberOfCalls(t, "SendMetadata", 1)
			}

			i.getPayload = func() marshaler.JSONMarshaler { return &testPayload{} }
			serializerMock.On("SendMetadata", &testPayload{}).Return(nil).Once()
			i.Refresh()
			i.collect(context.Background())
			assert.False(t, i.RefreshTriggered())
			serializerMock.AssertExpectations(t)
		})
	}
}

func TestReadinessGatesPayloadGeneration(t *testing.T) {
	for _, firstRunDelay := range []int{0, 3600} {
		t.Run((time.Duration(firstRunDelay) * time.Second).String(), func(t *testing.T) {
			i := getTestInventoryPayload(t, map[string]any{"inventories_first_run_delay": firstRunDelay})
			serializerMock := i.serializer.(*serializermock.MetricSerializer)
			serializerMock.On("SendMetadata", &testPayload{}).Return(nil).Times(3)
			builds := 0
			i.getPayload = func() marshaler.JSONMarshaler {
				builds++
				return &testPayload{}
			}

			// Reclosing after publication protects subsequent updates as well as startup.
			for cycle := range 2 {
				i.SetReady(false)
				require.NotNil(t, i.MetadataProvider().Callback, "readiness must not unregister the provider")
				lastCollect := i.LastCollect
				for _, pendingRefresh := range []bool{false, true} {
					i.forceRefresh.Store(pendingRefresh)
					assert.Equal(t, i.MinInterval, i.collect(context.Background()))
					i.Submit()
					data, err := i.GetAsJSON()
					assert.EqualError(t, err, "inventory metadata is not ready")
					assert.Nil(t, data)
					assert.Equal(t, pendingRefresh, i.RefreshTriggered())
					assert.Equal(t, lastCollect, i.LastCollect)
					assert.Equal(t, 2*cycle, builds)
					serializerMock.AssertNumberOfCalls(t, "SendMetadata", cycle)
				}

				i.SetReady(true)
				assert.Equal(t, lastCollect, i.LastCollect, "opening does not collect")
				assert.True(t, i.RefreshTriggered(), "opening does not consume refresh")
				i.Submit() // Immediate even with a large first-run delay or a recent collection.
				assert.False(t, i.RefreshTriggered())
				assert.False(t, i.LastCollect.IsZero())
				data, err := i.GetAsJSON()
				require.NoError(t, err)
				assert.JSONEq(t, `{"test": true}`, string(data))
				serializerMock.AssertNumberOfCalls(t, "SendMetadata", cycle+1)
			}

			// A reopened provider can also consume the pending refresh on its next poll.
			i.firstRunDelay = 0
			i.SetReady(false)
			i.Refresh()
			i.SetReady(true)
			assert.Equal(t, i.MinInterval, i.collect(context.Background()))
			assert.False(t, i.RefreshTriggered())
			assert.Equal(t, 5, builds)
			serializerMock.AssertNumberOfCalls(t, "SendMetadata", 3)
		})
	}
}

func TestSetReadyClosingBarrier(t *testing.T) {
	for _, operation := range []string{"collect", "Submit", "GetAsJSON"} {
		for _, stage := range []string{"generation", "enqueue"} {
			if operation == "GetAsJSON" && stage == "enqueue" {
				continue
			}
			t.Run(operation+"/"+stage, func(t *testing.T) {
				i := getTestInventoryPayload(t, map[string]any{"inventories_first_run_delay": 0})
				entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
				block := func() {
					close(entered)
					select {
					case <-release:
					case <-time.After(5 * time.Second):
						t.Error("timed out waiting to release in-flight payload")
					}
					close(finished)
				}
				i.getPayload = func() marshaler.JSONMarshaler {
					if stage == "generation" {
						block()
					}
					return &testPayload{}
				}
				serializerMock := i.serializer.(*serializermock.MetricSerializer)
				if operation != "GetAsJSON" {
					serializerMock.On("SendMetadata", &testPayload{}).Run(func(mock.Arguments) {
						if stage == "enqueue" {
							block()
						}
					}).Return(nil).Once()
				}
				operationDone := make(chan struct{})
				go func() {
					defer close(operationDone)
					switch operation {
					case "collect":
						i.collect(context.Background())
					case "Submit":
						i.Submit()
					case "GetAsJSON":
						_, err := i.GetAsJSON()
						assert.NoError(t, err)
					}
				}()
				waitForInventorySignal(t, entered)
				// Check the shared mutex is held throughout generation AND enqueue,
				// without relying on how soon the closing goroutine is scheduled.
				locked := i.m.TryLock()
				if locked {
					i.m.Unlock()
				}
				assert.False(t, locked, "payload work must hold the readiness mutex")

				closing, closed := make(chan struct{}), make(chan struct{})
				go func() {
					close(closing)
					i.SetReady(false)
					select {
					case <-finished:
					default:
						t.Error("closing returned before in-flight payload work finished")
					}
					close(closed)
				}()
				waitForInventorySignal(t, closing)
				select {
				case <-closed:
					t.Error("closing completed while payload work was blocked")
				default:
				}
				close(release)
				waitForInventorySignal(t, operationDone)
				waitForInventorySignal(t, closed)

				i.getPayload = func() marshaler.JSONMarshaler {
					t.Fatal("closed gate must not generate a payload")
					return nil
				}
				i.Refresh()
				i.collect(context.Background())
				i.Submit()
				_, err := i.GetAsJSON()
				assert.EqualError(t, err, "inventory metadata is not ready")
			})
		}
	}
}

func waitForInventorySignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for inventory operation")
	}
}

func TestCollectEmptyPayload(t *testing.T) {
	i := getEmptyInventoryPayload(t, nil)

	// Ensure calls to collect do not fail the check for createdAt
	i.createdAt = time.Now().Add(-2 * time.Minute)
	// Make sure the minInterval between two payload has expired
	i.LastCollect = time.Now().Add(-1 * time.Hour)

	now := time.Now()
	interval := i.collect(context.Background())
	assert.Equal(t, defaultMinInterval, interval)
	assert.False(t, i.LastCollect.Before(now))
	i.serializer.(*serializermock.MetricSerializer).AssertNotCalled(t, "SendMetadata")
}
