// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

func TestMain(m *testing.M) {
	// setting the hostname cache saves about 1s when starting the metric agent
	cacheKey := cache.BuildAgentKey("hostname")
	cache.Cache.Set(cacheKey, hostname.Data{}, cache.NoExpiration)
	os.Exit(m.Run())
}

func TestStartDoesNotBlock(t *testing.T) {
	config.DetectFeatures()
	metricAgent := &ServerlessMetricAgent{}
	defer metricAgent.Stop()
	metricAgent.Start(10*time.Second, &MetricConfig{}, &MetricDogStatsD{})
	assert.NotNil(t, metricAgent.Demux)
	assert.True(t, metricAgent.IsReady())
}

type ValidMetricConfigMocked struct {
}

func (m *ValidMetricConfigMocked) GetMultipleEndpoints() (map[string][]string, error) {
	return map[string][]string{"http://localhost:8888": {"value"}}, nil
}

type InvalidMetricConfigMocked struct {
}

func (m *InvalidMetricConfigMocked) GetMultipleEndpoints() (map[string][]string, error) {
	return nil, fmt.Errorf("error")
}

func TestStartInvalidConfig(t *testing.T) {
	metricAgent := &ServerlessMetricAgent{}
	defer metricAgent.Stop()
	metricAgent.Start(1*time.Second, &InvalidMetricConfigMocked{}, &MetricDogStatsD{})
	assert.False(t, metricAgent.IsReady())
}

type MetricDogStatsDMocked struct {
}

func (m *MetricDogStatsDMocked) NewServer(demux aggregator.Demultiplexer) (*dogstatsd.Server, error) {
	return nil, fmt.Errorf("error")
}

func TestStartInvalidDogStatsD(t *testing.T) {
	metricAgent := &ServerlessMetricAgent{}
	defer metricAgent.Stop()
	metricAgent.Start(1*time.Second, &MetricConfig{}, &MetricDogStatsDMocked{})
	assert.False(t, metricAgent.IsReady())
}

func TestStartWithProxy(t *testing.T) {
	originalValues := config.Datadog.GetStringSlice(statsDMetricBlocklistKey)
	defer config.Datadog.Set(statsDMetricBlocklistKey, originalValues)
	config.Datadog.Set(statsDMetricBlocklistKey, []string{})

	t.Setenv(proxyEnabledEnvVar, "true")

	metricAgent := &ServerlessMetricAgent{}
	defer metricAgent.Stop()
	metricAgent.Start(10*time.Second, &MetricConfig{}, &MetricDogStatsD{})

	expected := []string{
		invocationsMetric,
		ErrorsMetric,
	}

	setValues := config.Datadog.GetStringSlice(statsDMetricBlocklistKey)
	assert.Equal(t, expected, setValues)
}
func TestRaceFlushVersusAddSample(t *testing.T) {

	config.DetectFeatures()

	metricAgent := &ServerlessMetricAgent{}
	defer metricAgent.Stop()
	metricAgent.Start(10*time.Second, &ValidMetricConfigMocked{}, &MetricDogStatsD{})

	assert.NotNil(t, metricAgent.Demux)

	server := http.Server{
		Addr: "localhost:8888",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(10 * time.Millisecond)
		}),
	}
	defer server.Close()

	go func() {
		err := server.ListenAndServe()
		if !errors.Is(err, http.ErrServerClosed) {
			panic(err)
		}
	}()

	go func() {
		for i := 0; i < 1000; i++ {
			n := rand.Intn(10)
			time.Sleep(time.Duration(n) * time.Microsecond)
			go SendTimeoutEnhancedMetric([]string{"tag0:value0", "tag1:value1"}, metricAgent.Demux)
		}
	}()

	go func() {
		for i := 0; i < 1000; i++ {
			n := rand.Intn(10)
			time.Sleep(time.Duration(n) * time.Microsecond)
			go metricAgent.Flush()
		}
	}()

	time.Sleep(2 * time.Second)
}

func TestBuildMetricBlocklist(t *testing.T) {
	userProvidedBlocklist := []string{
		"user.defined.a",
		"user.defined.b",
	}
	expected := []string{
		"user.defined.a",
		"user.defined.b",
		invocationsMetric,
	}
	result := buildMetricBlocklist(userProvidedBlocklist)
	assert.Equal(t, expected, result)
}

func TestBuildMetricBlocklistForProxy(t *testing.T) {
	userProvidedBlocklist := []string{
		"user.defined.a",
		"user.defined.b",
	}
	expected := []string{
		"user.defined.a",
		"user.defined.b",
		invocationsMetric,
		ErrorsMetric,
	}
	result := buildMetricBlocklistForProxy(userProvidedBlocklist)
	assert.Equal(t, expected, result)
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

func TestRaceFlushVersusParsePacket(t *testing.T) {
	port, err := getAvailableUDPPort()
	require.NoError(t, err)
	config.Datadog.SetDefault("dogstatsd_port", port)

	opts := aggregator.DefaultAgentDemultiplexerOptions(nil)
	opts.FlushInterval = 10 * time.Millisecond
	opts.DontStartForwarders = true
	demux := aggregator.InitAndStartServerlessDemultiplexer(nil, time.Second*1000)

	s, err := dogstatsd.NewServer(demux, true)
	require.NoError(t, err, "cannot start DSD")
	defer s.Stop()

	url := fmt.Sprintf("127.0.0.1:%d", config.Datadog.GetInt("dogstatsd_port"))
	conn, err := net.Dial("udp", url)
	require.NoError(t, err, "cannot connect to DSD socket")
	defer conn.Close()

	finish := &sync.WaitGroup{}
	finish.Add(2)

	go func(wg *sync.WaitGroup) {
		for i := 0; i < 1000; i++ {
			conn.Write([]byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2"))
			time.Sleep(10 * time.Nanosecond)
		}
		finish.Done()
	}(finish)

	go func(wg *sync.WaitGroup) {
		for i := 0; i < 1000; i++ {
			s.ServerlessFlush()
		}
		finish.Done()
	}(finish)

	finish.Wait()
}
