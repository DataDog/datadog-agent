package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"text/template"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
	ddstatus "github.com/DataDog/datadog-agent/pkg/status"
)

type statusServer struct {
	shutdownWg                      *sync.WaitGroup
	coreStatusServer, expvarsServer *http.Server
}

func (s *statusServer) stop() error {
	err := s.coreStatusServer.Shutdown(context.Background())
	if err != nil {
		return err
	}

	err = s.expvarsServer.Shutdown(context.Background())
	if err != nil {
		return err
	}

	s.shutdownWg.Wait()
	return nil
}

func startTestServer(t *testing.T, cfg config.Config, expectedStatus status) statusServer {
	var serverWg sync.WaitGroup
	serverWg.Add(2)

	statusMux := http.NewServeMux()
	statusMux.HandleFunc("/agent/status", func(w http.ResponseWriter, _ *http.Request) {
		b, err := json.Marshal(expectedStatus.Core)
		require.NoError(t, err)

		_, err = w.Write(b)
		require.NoError(t, err)
	})
	statusEndpoint := fmt.Sprintf("localhost:%d", cfg.GetInt("process_config.cmd_port"))
	coreStatusServer := http.Server{Addr: statusEndpoint, Handler: statusMux}
	statusListener, err := net.Listen("tcp", statusEndpoint)
	require.NoError(t, err)
	go func() {
		_ = coreStatusServer.Serve(statusListener)
		serverWg.Done()
	}()

	expvarMux := http.NewServeMux()
	expvarMux.HandleFunc("/debug/vars", func(w http.ResponseWriter, _ *http.Request) {
		b, err := json.Marshal(expectedStatus.Expvars)
		require.NoError(t, err)

		_, err = w.Write(b)
		require.NoError(t, err)
	})
	expvarEndpoint := fmt.Sprintf("localhost:%d", cfg.GetInt("process_config.expvar_port"))
	expvarsServer := http.Server{Addr: expvarEndpoint, Handler: expvarMux}
	expvarsListener, err := net.Listen("tcp", expvarEndpoint)
	require.NoError(t, err)
	go func() {
		_ = expvarsServer.Serve(expvarsListener)
		serverWg.Done()
	}()

	return statusServer{coreStatusServer: &coreStatusServer, expvarsServer: &expvarsServer, shutdownWg: &serverWg}
}

func TestStatus(t *testing.T) {
	testTime := time.Now()
	expectedStatus := status{
		Date:    float64(testTime.UnixNano()),
		Core:    coreStatus{},
		Expvars: processExpvars{},
	}

	cfg := config.Mock()
	cfg.Set("process_config.expvar_port", 8081)
	cfg.Set("process_config.cmd_port", 8082)
	server := startTestServer(t, cfg, expectedStatus)

	var statusBuilder, expectedStatusBuilder strings.Builder

	// Build what the expected status should be
	tpl, err := template.New("").Funcs(ddstatus.Textfmap()).Parse(statusTemplate)
	require.NoError(t, err)
	err = tpl.Execute(&expectedStatusBuilder, expectedStatus)
	require.NoError(t, err)

	// Build the actual status
	getAndWriteStatus(&statusBuilder, overrideTime(testTime))

	assert.Equal(t, expectedStatusBuilder.String(), statusBuilder.String())

	err = server.stop()
	require.NoError(t, err)
}

func TestNotRunning(t *testing.T) {
	var b strings.Builder
	getAndWriteStatus(&b)

	assert.Equal(t, notRunning, b.String())
}
