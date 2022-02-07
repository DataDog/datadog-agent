package app

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
)

type statusServer struct {
	statusListner, expvarListner net.Listener
}

func (s *statusServer) stop() error {
	err := s.statusListner.Close()
	if err != nil {
		return err
	}

	return s.expvarListner.Close()
}

func startTestServer(t *testing.T, cfg config.Config, expectedStatus status) statusServer {
	statusMux := http.NewServeMux()
	statusMux.HandleFunc("/agent/status", func(w http.ResponseWriter, _ *http.Request) {
		b, err := json.Marshal(expectedStatus)
		require.NoError(t, err)

		_, err = w.Write(b)
		require.NoError(t, err)
	})
	statusEndpoint := fmt.Sprintf("localhost:%d", cfg.GetInt("process_config.cmd_port"))
	statusListener, err := net.Listen("tcp", statusEndpoint)
	require.NoError(t, err)
	go func() {
		_ = http.Serve(statusListener, statusMux)
	}()

	expvarMux := http.NewServeMux()
	expvarMux.HandleFunc("/debug/vars", func(w http.ResponseWriter, _ *http.Request) {
		b, err := json.Marshal(expectedStatus)
		require.NoError(t, err)

		_, err = w.Write(b)
		require.NoError(t, err)
	})
	expvarEndpoint := fmt.Sprintf("localhost:%d", cfg.GetInt("process_config.expvar_port"))
	expvarsListener, err := net.Listen("tcp", expvarEndpoint)
	go func() {
		_ = http.Serve(expvarsListener, expvarMux)
	}()
	return statusServer{statusListener, expvarsListener}
}

func TestStatus(t *testing.T) {
	testTime := time.Now()
	expectedStatus := status{
		Date:    testTime.Format(time.RFC850),
		Core:    coreStatus{},
		Expvars: processExpvars{},
	}

	cfg := config.Mock()
	cfg.Set("process_config.expvar_port", 8081)
	cfg.Set("process_config.cmd_port", 8082)
	server := startTestServer(t, cfg, expectedStatus)
	defer server.stop()
	time.Sleep(2 * time.Second) // Wait 2 seconds for the server to start up.

	var statusBuilder, expectedStatusBuilder strings.Builder

	// Build what the expected status should be
	tpl, err := template.New("").Parse(statusTemplate)
	require.NoError(t, err)
	err = tpl.Execute(&expectedStatusBuilder, expectedStatus)
	require.NoError(t, err)

	// Build the actual status
	getAndWriteStatus(&statusBuilder, overrideTime(testTime))

	assert.Equal(t, expectedStatusBuilder.String(), statusBuilder.String())
}

func TestNotRunning(t *testing.T) {
	var b strings.Builder
	getAndWriteStatus(&b)

	assert.Equal(t, notRunning, b.String())
}
