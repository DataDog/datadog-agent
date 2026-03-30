// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && test

package socketcontention

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/server/testutil"
)

func TestRunEmitsMetricsAndSkipsZeroCountEntries(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	socketPath, server := startSocketContentionTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/debug/stats":
			_, _ = w.Write([]byte(`{}`))
		case "/socket_contention/check":
			_, _ = w.Write([]byte(`[
				{
					"object_kind":"socket",
					"socket_type":"stream",
					"family":"inet",
					"protocol":"tcp",
					"lock_subtype":"sk_lock",
					"cgroup_id":1234,
					"flags":3,
					"total_time_ns":100,
					"min_time_ns":10,
					"max_time_ns":80,
					"count":4
				},
				{
					"object_kind":"unknown",
					"socket_type":"unknown",
					"family":"unknown",
					"protocol":"unknown",
					"lock_subtype":"unknown",
					"cgroup_id":0,
					"flags":0,
					"total_time_ns":0,
					"min_time_ns":0,
					"max_time_ns":0,
					"count":0
				}
			]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	server.Start()
	t.Cleanup(server.Close)

	pkgconfigsetup.SystemProbe().Set("system_probe_config.sysprobe_socket", socketPath, configmodel.SourceAgentRuntime)

	checkGeneric := newCheck(fakeTagger)
	check, ok := checkGeneric.(*Check)
	require.True(t, ok)

	senderManager := mocksender.CreateDefaultDemultiplexer()
	require.NoError(t, check.Configure(senderManager, integration.FakeConfigHash, []byte{}, []byte{}, "test", "provider"))

	mockSender := mocksender.NewMockSenderWithSenderManager(check.ID(), senderManager)
	mockSender.SetupAcceptAll()

	check.sysProbeClient = client.GetCheckClient(client.WithSocketPath(socketPath))
	check.cgroupReader = nil

	require.NoError(t, check.Run())

	expectedTags := []string{
		"object_kind:socket",
		"socket_type:stream",
		"socket_family:inet",
		"protocol:tcp",
		"lock_subtype:sk_lock",
	}
	mockSender.AssertMetric(t, "Gauge", "socket_contention.contention_count", float64(4), "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "socket_contention.contention_total_ns", float64(100), "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "socket_contention.contention_max_ns", float64(80), "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "socket_contention.contention_min_ns", float64(10), "", expectedTags)
	mockSender.AssertNumberOfCalls(t, "Gauge", 4)
	mockSender.AssertCalled(t, "Commit")
}

func TestGetContainerTagsReturnsNilWithoutReaderOrCgroupID(t *testing.T) {
	check := &Check{}
	require.Nil(t, check.getContainerTags(0))

	check.cgroupReader = nil
	require.Nil(t, check.getContainerTags(1234))
}

func startSocketContentionTestServer(t *testing.T, handler http.Handler) (string, *httptest.Server) {
	t.Helper()
	socketPath := testutil.SystemProbeSocketPath(t, "socket-contention")
	server, err := testutil.NewSystemProbeTestServer(handler, socketPath)
	require.NoError(t, err)
	return socketPath, server
}
