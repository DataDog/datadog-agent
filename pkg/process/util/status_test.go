package util

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/config"
	"net"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type statusServer struct {
	shutdownWg   *sync.WaitGroup
	expVarServer *http.Server
}

func (s *statusServer) stop() error {
	err := s.expVarServer.Shutdown(context.Background())
	if err != nil {
		return err
	}

	s.shutdownWg.Wait()
	return nil
}

func startTestServer(t *testing.T, cfg config.Config, expectedExpVars ProcessExpvars) statusServer {
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
	expVarsServer := http.Server{Addr: expCarEndpoint, Handler: expVarMux}
	expVarsListener, err := net.Listen("tcp", expVarEndpoint)
	require.NoError(t, err)
	go func() {
		_ = expVarsServer.Serve(expVarsListener)
		serverWg.Done()
	}()

	return statusServer{expVarServer: &expVarsServer, shutdownWg: &serverWg}
}

func TestGetStatus(t *testing.T) {

}
