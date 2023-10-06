// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package status

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func fakeExpVarServer(t *testing.T, expVars ProcessExpvars) *httptest.Server {
	handler := func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		b, err := json.Marshal(expVars)
		require.NoError(t, err)

		_, err = w.Write(b)
		require.NoError(t, err)
	}

	return httptest.NewServer(http.HandlerFunc(handler))
}

func TestGetStatus(t *testing.T) {
	testTime := time.Now()

	expectedExpVars := ProcessExpvars{
		Pid:           1,
		Uptime:        time.Now().Add(-time.Hour).Nanosecond(),
		EnabledChecks: []string{"process", "rtprocess"},
		MemStats: MemInfo{
			Alloc: 1234,
		},
		Endpoints: map[string][]string{
			"https://process.datadoghq.com": {
				"fakeAPIKey",
			},
		},
		LastCollectTime:                 "2022-02-011 10:10:00",
		DockerSocket:                    "/var/run/docker.sock",
		ProcessCount:                    30,
		ContainerCount:                  2,
		ProcessQueueSize:                1,
		RTProcessQueueSize:              3,
		ConnectionsQueueSize:            4,
		PodQueueSize:                    5,
		ProcessQueueBytes:               2 * 1024,
		RTProcessQueueBytes:             512,
		ConnectionsQueueBytes:           8 * 1024,
		PodQueueBytes:                   4 * 1024,
		SystemProbeProcessModuleEnabled: true,
		LanguageDetectionEnabled:        true,
		WlmExtractorCacheSize:           36,
		WlmExtractorStaleDiffs:          1,
		WlmExtractorDiffsDropped:        2,
	}

	// Feature detection needs to run before host methods are called. During runtime, feature detection happens
	// when the datadog.yaml file is loaded
	cfg := ddconfig.Mock(t)
	ddconfig.SetFeatures(t)
	cfg.Set("hostname", "test") // Prevents panic since feature detection has not run
	cfg.Set("language_detection.enabled", true)

	hostnameData, err := hostname.GetWithProvider(context.Background())
	var metadata *host.Payload
	if err != nil {
		metadata = host.GetPayloadFromCache(context.Background(), hostname.Data{Hostname: "unknown", Provider: "unknown"})
	} else {
		metadata = host.GetPayloadFromCache(context.Background(), hostnameData)
	}

	expectedStatus := &Status{
		Date: float64(testTime.UnixNano()),
		Core: CoreStatus{
			AgentVersion: version.AgentVersion,
			GoVersion:    runtime.Version(),
			Arch:         runtime.GOARCH,
			Config: ConfigStatus{
				LogLevel: cfg.GetString("log_level"),
			},
			Metadata: *metadata,
		},
		Expvars: expectedExpVars,
	}

	expVarSrv := fakeExpVarServer(t, expectedExpVars)
	defer expVarSrv.Close()

	stats, err := GetStatus(cfg, expVarSrv.URL)
	require.NoError(t, err)

	OverrideTime(testTime)(stats)
	assert.Equal(t, expectedStatus, stats)
}
