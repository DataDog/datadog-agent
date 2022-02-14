package util

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type expVarServer struct {
	shutdownWg *sync.WaitGroup
	server     *http.Server
}

func (s *expVarServer) stop() error {
	err := s.server.Shutdown(context.Background())
	if err != nil {
		return err
	}

	s.shutdownWg.Wait()
	return nil
}

func startTestServer(t *testing.T, cfg config.Config, expectedExpVars ProcessExpvars) expVarServer {
	var serverWg sync.WaitGroup
	serverWg.Add(1)

	expVarMux := http.NewServeMux()
	expVarMux.HandleFunc("/debug/vars", func(w http.ResponseWriter, _ *http.Request) {
		b, err := json.Marshal(expectedExpVars)
		require.NoError(t, err)

		_, err = w.Write(b)
		require.NoError(t, err)
	})
	expVarEndpoint := fmt.Sprintf("localhost:%d", cfg.GetInt("process_config.expvar_port"))
	expVarsServer := http.Server{Addr: expVarEndpoint, Handler: expVarMux}
	expVarsListener, err := net.Listen("tcp", expVarEndpoint)
	require.NoError(t, err)
	go func() {
		_ = expVarsServer.Serve(expVarsListener)
		serverWg.Done()
	}()

	return expVarServer{server: &expVarsServer, shutdownWg: &serverWg}
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
		LastCollectTime:     "2022-02-011 10:10:00",
		DockerSocket:        "/var/run/docker.sock",
		ProcessCount:        30,
		ContainerCount:      2,
		ProcessQueueSize:    1,
		RTProcessQueueSize:  3,
		PodQueueSize:        4,
		ProcessQueueBytes:   2 * 1024,
		RTProcessQueueBytes: 512,
		PodQueueBytes:       4 * 1024,
	}

	// Feature detection needs to run before host methods are called. During runtime, feature detection happens
	// when the datadog.yaml file is loaded
	cfg := ddconfig.Mock()
	ddconfig.DetectFeatures()

	hostnameData, err := util.GetHostnameData(context.Background())
	var metadata *host.Payload
	if err != nil {
		metadata = host.GetPayloadFromCache(context.Background(), util.HostnameData{Hostname: "unknown", Provider: "unknown"})
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
				LogLevel: ddconfig.Datadog.GetString("log_level"),
			},
			Metadata: *metadata,
		},
		Expvars: expectedExpVars,
	}

	// Use different port in case the host is running a real agent
	cfg.Set("process_config.expvar_port", 8081)

	expVarSrv := startTestServer(t, cfg, expectedExpVars)
	defer func() {
		err := expVarSrv.stop()
		require.NoError(t, err)
	}()

	stats, err := GetStatus()
	require.NoError(t, err)

	OverrideTime(testTime)(stats)
	assert.Equal(t, expectedStatus, stats)
}
