// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package vdi

import (
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/vdi/model"
)

type fakeAVDInventory struct {
	response model.InventoryResponse
	err      error
	onGet    func()
}

func (f fakeAVDInventory) Get() (model.InventoryResponse, error) {
	if f.onGet != nil {
		f.onGet()
	}
	return f.response, f.err
}

func configuredAVDCheck(t *testing.T, config string) (*checkImpl, *mocksender.MockSender) {
	t.Helper()
	c := newCheck().(*checkImpl)
	raw := integration.Data("provider: azure_virtual_desktop\n" + config)
	c.BuildID(integration.FakeConfigHash, raw, nil)
	s := mocksender.NewMockSender(t, c.ID())
	s.On("FinalizeCheckServiceTag").Return().Once()
	require.NoError(t, c.Configure(s.GetSenderManager(), integration.FakeConfigHash,
		raw, nil, "test", "test"))
	require.Nil(t, c.query, "AVD must not require a DCV PDH query")
	s.On("Commit").Return().Once()
	return c, s
}

func TestAVDMetricsIncludeDisconnectedSessions(t *testing.T) {
	c, s := configuredAVDCheck(t, "")
	now := time.Now().UTC()
	logon := now.Add(-time.Hour)
	lastInput := now.Add(-time.Minute)
	c.now = func() time.Time { return now }
	c.inventoryClient = fakeAVDInventory{response: model.InventoryResponse{
		CollectedAt: now,
		Providers: map[string]model.ProviderInventory{model.ProviderAzureVirtualDesktop: {
			SourceStatus: model.SourceStatus{Status: model.StatusOK},
			Sessions: []model.Session{
				{ID: "3", User: "EXAMPLE\\alice", State: "active", LogonAt: &logon, LastInputAt: &lastInput},
				{ID: "7", User: "EXAMPLE\\bob", State: "disconnected"},
			},
		}},
	}}
	base := []string{"vdi_provider:azure_virtual_desktop", "vdi_protocol:rdp"}
	alice := append(append([]string{}, base...), "vdi_session_id:3", "vdi_session_state:active", "vdi_session_user:EXAMPLE\\alice")
	bob := append(append([]string{}, base...), "vdi_session_id:7", "vdi_session_state:disconnected", "vdi_session_user:EXAMPLE\\bob")
	for name, value := range map[string]float64{"count": 2, "active": 1, "disconnected": 1} {
		s.On("Gauge", "vdi.session."+name, value, "", base).Return().Once()
	}
	s.On("Gauge", "vdi.session.present", 1.0, "", alice).Return().Once()
	s.On("Gauge", "vdi.session.present", 1.0, "", bob).Return().Once()
	s.On("Gauge", "vdi.session.duration", 3600.0, "", alice).Return().Once()
	s.On("Gauge", "vdi.session.idle_time", 60.0, "", alice).Return().Once()
	s.On("ServiceCheck", "vdi.session_enrichment.health", servicecheck.ServiceCheckOK, "", base, "").Return().Once()
	require.NoError(t, c.Run())
	s.AssertExpectations(t)
	s.AssertNumberOfCalls(t, "Gauge", 7)
	s.AssertNumberOfCalls(t, "Commit", 1)
}

func TestAVDFailedAndStaleInventoryDoesNotEmitZeroCountsOrOldUsers(t *testing.T) {
	for _, test := range []struct {
		name   string
		age    time.Duration
		err    error
		status string
	}{
		{name: "transport failure", err: errors.New("named pipe unavailable"), status: model.StatusOK},
		{name: "long localized failure", err: errors.New(strings.Repeat("界", 400)), status: model.StatusOK},
		{name: "WTS failure", status: model.StatusError},
		{name: "stale snapshot", age: 2 * time.Minute, status: model.StatusOK},
		{name: "future snapshot", age: -time.Minute, status: model.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, s := configuredAVDCheck(t, "")
			now := time.Now().UTC()
			c.now = func() time.Time { return now }
			c.startedAt = now.Add(-10 * time.Minute)
			c.inventoryClient = fakeAVDInventory{err: test.err, response: model.InventoryResponse{
				CollectedAt: now.Add(-test.age),
				Providers: map[string]model.ProviderInventory{model.ProviderAzureVirtualDesktop: {
					SourceStatus: model.SourceStatus{Status: test.status},
					Sessions:     []model.Session{{ID: "3", User: "stale-user", State: "active"}},
				}},
			}}
			s.On("ServiceCheck", "vdi.session_enrichment.health", servicecheck.ServiceCheckCritical, "", c.baseTags(), mock.MatchedBy(func(message string) bool {
				return len(message) <= 1024 && utf8.ValidString(message)
			})).Return().Once()
			require.NoError(t, c.Run())
			require.NotEmpty(t, c.GetWarnings(), "inventory failures must appear in agent status")
			s.AssertNumberOfCalls(t, "Gauge", 0)
			s.AssertExpectations(t)
		})
	}
}

func TestAVDInventoryFreshnessUsesResponseTime(t *testing.T) {
	for _, test := range []struct {
		name            string
		collectedOffset time.Duration
		expectedHealth  servicecheck.ServiceCheckStatus
		expectedGauges  int
	}{
		{name: "new snapshot after slow request", collectedOffset: 10 * time.Second, expectedHealth: servicecheck.ServiceCheckOK, expectedGauges: 3},
		{name: "snapshot expires during request", collectedOffset: -55 * time.Second, expectedHealth: servicecheck.ServiceCheckCritical},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, s := configuredAVDCheck(t, "")
			now := time.Now().UTC()
			c.now = func() time.Time { return now }
			c.startedAt = now.Add(-10 * time.Minute)
			c.inventoryClient = fakeAVDInventory{
				response: model.InventoryResponse{
					CollectedAt: now.Add(test.collectedOffset),
					Providers: map[string]model.ProviderInventory{
						model.ProviderAzureVirtualDesktop: {SourceStatus: model.SourceStatus{Status: model.StatusOK}},
					},
				},
				onGet: func() { now = now.Add(10 * time.Second) },
			}
			if test.expectedGauges > 0 {
				for _, metric := range []string{"count", "active", "disconnected"} {
					s.On("Gauge", "vdi.session."+metric, 0.0, "", c.baseTags()).Return().Once()
				}
			}
			s.On("ServiceCheck", "vdi.session_enrichment.health", test.expectedHealth, "", c.baseTags(), mock.Anything).Return().Once()
			require.NoError(t, c.Run())
			s.AssertNumberOfCalls(t, "Gauge", test.expectedGauges)
			s.AssertExpectations(t)
		})
	}
}

func TestAVDTagControlsAndEmptyInventory(t *testing.T) {
	for _, test := range []struct {
		name, config   string
		sessions       []model.Session
		expectedGauges int
	}{
		{name: "empty host", expectedGauges: 3},
		{name: "session tags disabled", config: "collect_session_tags: false\n", sessions: []model.Session{{ID: "3", User: "alice", State: "active"}}, expectedGauges: 3},
		{name: "user tags disabled", config: "collect_user_tags: false\n", sessions: []model.Session{{ID: "3", User: "alice", State: "active"}}, expectedGauges: 4},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, s := configuredAVDCheck(t, test.config)
			now := time.Now().UTC()
			c.now = func() time.Time { return now }
			c.inventoryClient = fakeAVDInventory{response: model.InventoryResponse{
				CollectedAt: now,
				Providers:   map[string]model.ProviderInventory{model.ProviderAzureVirtualDesktop: {SourceStatus: model.SourceStatus{Status: model.StatusOK}, Sessions: test.sessions}},
			}}
			s.On("Gauge", mock.Anything, mock.Anything, "", mock.MatchedBy(func(tags []string) bool {
				for _, tag := range tags {
					if tag == "vdi_session_user:alice" {
						return false
					}
				}
				return true
			})).Return()
			s.On("ServiceCheck", "vdi.session_enrichment.health", servicecheck.ServiceCheckOK, "", c.baseTags(), "").Return().Once()
			require.NoError(t, c.Run())
			s.AssertNumberOfCalls(t, "Gauge", test.expectedGauges)
			s.AssertExpectations(t)
		})
	}
}

func TestAVDPartialInventoryEmitsKnownSessionsWithoutAggregateCounts(t *testing.T) {
	c, s := configuredAVDCheck(t, "")
	now := time.Now().UTC()
	future := now.Add(time.Minute)
	c.now = func() time.Time { return now }
	c.inventoryClient = fakeAVDInventory{response: model.InventoryResponse{
		CollectedAt: now,
		Providers: map[string]model.ProviderInventory{model.ProviderAzureVirtualDesktop: {
			SourceStatus: model.SourceStatus{Status: model.StatusPartial, Error: "one protocol query failed"},
			Sessions:     []model.Session{{ID: "3", State: "active", LogonAt: &future, LastInputAt: &future}},
		}},
	}}
	tags := append(c.baseTags(), "vdi_session_id:3", "vdi_session_state:active")
	s.On("Gauge", "vdi.session.present", 1.0, "", tags).Return().Once()
	s.On("ServiceCheck", "vdi.session_enrichment.health", servicecheck.ServiceCheckWarning, "", c.baseTags(), "one protocol query failed").Return().Once()
	require.NoError(t, c.Run())
	s.AssertNumberOfCalls(t, "Gauge", 1)
	require.NotEmpty(t, c.GetWarnings(), "partial inventory must appear in agent status")
	s.AssertExpectations(t)
}
