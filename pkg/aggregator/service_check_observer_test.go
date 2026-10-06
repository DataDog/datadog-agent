// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	filterlistmock "github.com/DataDog/datadog-agent/comp/filterlist/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/healthcheck"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
)

type serviceCheckObservation struct {
	id     checkid.ID
	name   string
	status servicecheck.ServiceCheckStatus
}

type recordingServiceCheckObserver struct {
	calls []serviceCheckObservation
}

func (o *recordingServiceCheckObserver) ObserveServiceCheck(id checkid.ID, name string, status servicecheck.ServiceCheckStatus) {
	o.calls = append(o.calls, serviceCheckObservation{id: id, name: name, status: status})
}

func TestSenderServiceCheckObserverGuards(t *testing.T) {
	for _, id := range []checkid.ID{"", "http_check:instance"} {
		t.Run(string(id), func(t *testing.T) {
			s := initSender(id, "host")
			s.sender.ServiceCheck("http.can_connect", servicecheck.ServiceCheckOK, "", nil, "healthy")
			assert.Equal(t, servicecheck.ServiceCheckOK, (<-s.serviceCheckChan).Status)
			o := &recordingServiceCheckObserver{}
			s.sender.serviceCheckObserver = o
			s.sender.ServiceCheck("http.can_connect", servicecheck.ServiceCheckCritical, "", nil, "unhealthy")
			assert.Equal(t, servicecheck.ServiceCheckCritical, (<-s.serviceCheckChan).Status)
			if id == "" {
				assert.Empty(t, o.calls)
			} else {
				assert.Equal(t, []serviceCheckObservation{{id: id, name: "http.can_connect", status: servicecheck.ServiceCheckCritical}}, o.calls)
			}
		})
	}
}

type blockedRemediationDispatcher struct {
	started bool
	stopped bool
}

func (d *blockedRemediationDispatcher) Dispatch(ctx context.Context, _ checkid.ID, _ string, _ *integration.HealthCheckConfig) {
	d.started = true
	<-ctx.Done()
	d.stopped = true
}

func TestSenderServiceCheckObserverBackpressure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		id := checkid.ID(t.Name())
		healthcheck.Register(id, &integration.HealthCheckConfig{Enabled: true})
		defer healthcheck.Unregister(id)
		dispatcher := &blockedRemediationDispatcher{}
		o := healthcheck.NewObserver(dispatcher)
		defer o.Stop()
		s := initSender(id, "host")
		s.sender.serviceCheckObserver = o
		s.sender.ServiceCheck("http.can_connect", servicecheck.ServiceCheckOK, "", nil, "")
		<-s.serviceCheckChan
		s.sender.ServiceCheck("http.can_connect", servicecheck.ServiceCheckCritical, "", nil, "")
		<-s.serviceCheckChan
		synctest.Wait()
		require.True(t, dispatcher.started)
		for range 256 {
			s.sender.ServiceCheck("http.can_connect", servicecheck.ServiceCheckCritical, "", nil, "")
			assert.Equal(t, servicecheck.ServiceCheckCritical, (<-s.serviceCheckChan).Status)
		}
		assert.Positive(t, o.Dropped())
		o.Stop()
		assert.True(t, dispatcher.stopped)
	})
}

func TestServiceCheckObserverSetOnceAndSenderPropagation(t *testing.T) {
	first := &recordingServiceCheckObserver{}
	agg := &BufferedAggregator{checkItems: make(chan senderItem, 1)}
	agg.SetServiceCheckObserver(nil)
	agg.SetServiceCheckObserver(first)
	agg.SetServiceCheckObserver(&recordingServiceCheckObserver{})
	pool := &checkSenderPool{agg: agg, senders: make(map[checkid.ID]sender.Sender)}
	s, err := pool.mkSender("http_check:instance")
	require.NoError(t, err)
	assert.Same(t, first, s.(*checkSender).serviceCheckObserver)
}

func TestHealthCheckRemediationAggregatorFeatureFlag(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		name := "default disabled"
		if enabled {
			name = "enabled emits dry-run event"
		}
		t.Run(name, func(t *testing.T) {
			cfg := configmock.New(t)
			if enabled {
				cfg.SetInTest("health_check_remediation.enabled", true)
			}
			agg := NewBufferedAggregator(nil, nil, nil, nooptagger.NewComponent(), "test-host", DefaultFlushInterval, filterlistmock.NewMockFilterList())
			defer func() {
				go agg.run()
				agg.Stop()
			}()
			if !enabled {
				assert.Nil(t, agg.serviceCheckObserver)
				assert.Nil(t, agg.remediationObserver)
				return
			}
			require.NotNil(t, agg.remediationObserver)
			id := checkid.ID(t.Name())
			healthcheck.Register(id, &integration.HealthCheckConfig{
				Enabled:      true,
				ServiceCheck: "http.can_connect",
				Remediation:  integration.RemediationConfig{Steps: []integration.RemediationStep{{Command: "/bin/false"}}},
			})
			defer healthcheck.Unregister(id)
			pool := &checkSenderPool{agg: agg, senders: make(map[checkid.ID]sender.Sender)}
			s, err := pool.mkSender(id)
			require.NoError(t, err)
			s.ServiceCheck("http.can_connect", servicecheck.ServiceCheckOK, "", nil, "")
			s.ServiceCheck("http.can_connect", servicecheck.ServiceCheckCritical, "", nil, "")
			e := <-agg.eventIn
			assert.Equal(t, "health-check remediation (dry-run)", e.Title)
			assert.Equal(t, "test-host", e.Host)
			assert.Contains(t, e.Text, string(id))
			assert.Contains(t, e.Text, "http.can_connect")
			assert.Contains(t, e.Text, "/bin/false")
			assert.Contains(t, e.Tags, "remediation:dry-run")
		})
	}
}
