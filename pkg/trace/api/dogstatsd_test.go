package api

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentConfig "github.com/DataDog/datadog-agent/pkg/config"
)

func TestDogStatsDReverseProxy(t *testing.T) {
	testCases := []struct {
		name       string
		configFunc func(cfg *config.AgentConfig)
		errCode    int
	}{
		{
			"dogstatsd disabled",
			func(cfg *config.AgentConfig) {
				cfg.StatsdEnabled = false
			},
			http.StatusMethodNotAllowed,
		},
		{
			"bad statsd host",
			func(cfg *config.AgentConfig) {
				cfg.StatsdHost = "this is invalid"
			},
			http.StatusInternalServerError,
		},
		{
			"bad statsd port",
			func(cfg *config.AgentConfig) {
				cfg.StatsdPort = -1
			},
			http.StatusInternalServerError,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.New()
			tc.configFunc(cfg)
			receiver := newTestReceiverFromConfig(cfg)
			proxy := receiver.dogstatsdProxyHandler()
			require.NotNil(t, proxy)

			rec := httptest.NewRecorder()
			proxy.ServeHTTP(rec, httptest.NewRequest("POST", "/", nil))
			require.Equal(t, tc.errCode, rec.Code)
		})
	}

	t.Run("dogstatsd enabled (default)", func(t *testing.T) {
		cfg := config.New()
		receiver := newTestReceiverFromConfig(cfg)
		proxy := receiver.dogstatsdProxyHandler()
		require.NotNil(t, proxy)

		rec := httptest.NewRecorder()
		body := ioutil.NopCloser(bytes.NewBufferString("users.online:1|c|@0.5|#country:china"))
		proxy.ServeHTTP(rec, httptest.NewRequest("POST", "/", body))
	})
}

func TestDogStatsDReverseProxyEndToEnd(t *testing.T) {
	// This test is based on pkg/dogstatsd/server_test.go.
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	port, err := getAvailableUDPPort()
	if err != nil {
		t.Skip("Couldn't find available UDP port to run test. Skipping.")
	}
	agentConfig.Datadog.SetDefault("dogstatsd_port", port)
	agentConfig.Datadog.Set("dogstatsd_no_aggregation_pipeline", true)
	defer func() {
		agentConfig.Datadog.Set("dogstatsd_no_aggregation_pipeline", false)
	}()
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(10 * time.Millisecond)
	defer demux.Stop(false)
	s, err := dogstatsd.NewServer(demux, false)
	require.NoError(t, err, "cannot start DSD")
	defer s.Stop()

	cfg := config.New()
	cfg.StatsdHost = "127.0.0.1"
	cfg.StatsdPort = port
	receiver := newTestReceiverFromConfig(cfg)
	proxy := receiver.dogstatsdProxyHandler()
	require.NotNil(t, proxy)
	rec := httptest.NewRecorder()

	// Test metrics
	body := ioutil.NopCloser(bytes.NewBufferString("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2"))
	proxy.ServeHTTP(rec, httptest.NewRequest("POST", "/", body))
	require.Equal(t, http.StatusOK, rec.Code)

	samples, timedSamples := demux.WaitForSamples(time.Second * 2)
	require.Len(t, samples, 1)
	require.Len(t, timedSamples, 0)
	sample := samples[0]
	assert.NotNil(t, sample)
	assert.Equal(t, sample.Name, "daemon")
	assert.EqualValues(t, sample.Value, 666.0)
	assert.Equal(t, sample.Mtype, metrics.GaugeType)
	assert.ElementsMatch(t, sample.Tags, []string{"sometag1:somevalue1", "sometag2:somevalue2"})
	demux.Reset()

	// Test services
	body = ioutil.NopCloser(bytes.NewBufferString("_sc|agent.up|0|d:12345|h:localhost|m:this is fine|#sometag1:somevalyyue1,sometag2:somevalue2"))
	proxy.ServeHTTP(rec, httptest.NewRequest("POST", "/", body))
	require.Equal(t, http.StatusOK, rec.Code)
	_, serviceOut := demux.GetEventsAndServiceChecksChannels()
	select {
	case res := <-serviceOut:
		assert.NotNil(t, res)
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}
}

// getAvailableUDPPort requests a random port number and makes sure it is available
func getAvailableUDPPort() (int, error) {
	conn, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return -1, fmt.Errorf("can't find an available udp port: %s", err)
	}
	defer conn.Close()

	_, portString, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		return -1, fmt.Errorf("can't find an available udp port: %s", err)
	}
	portInt, err := strconv.Atoi(portString)
	if err != nil {
		return -1, fmt.Errorf("can't convert udp port: %s", err)
	}

	return portInt, nil
}
