// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartDoesNotBlock(t *testing.T) {
	config.DetectFeatures()
	metricAgent := &ServerlessMetricAgent{}
	defer metricAgent.Stop()
	metricAgent.Start(10*time.Second, &MetricConfig{}, &MetricDogStatsD{})
	assert.NotNil(t, metricAgent.GetMetricChannel())
	assert.True(t, metricAgent.IsReady())
	// allow some time to stop to avoid 'can't listen: listen udp 127.0.0.1:8125: bind: address already in use'
	time.Sleep(100 * time.Millisecond)
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
	// allow some time to stop to avoid 'can't listen: listen udp 127.0.0.1:8125: bind: address already in use'
	time.Sleep(100 * time.Millisecond)
}

type MetricDogStatsDMocked struct {
}

func (m *MetricDogStatsDMocked) NewServer(demux aggregator.Demultiplexer, extraTags []string) (*dogstatsd.Server, error) {
	return nil, fmt.Errorf("error")
}

func TestStartInvalidDogStatsD(t *testing.T) {
	metricAgent := &ServerlessMetricAgent{}
	defer metricAgent.Stop()
	metricAgent.Start(1*time.Second, &MetricConfig{}, &MetricDogStatsDMocked{})
	assert.False(t, metricAgent.IsReady())
	// allow some time to stop to avoid 'can't listen: listen udp 127.0.0.1:8125: bind: address already in use'
	time.Sleep(1 * time.Second)
}

func TestStartWithProxy(t *testing.T) {
	originalValues := config.Datadog.GetStringSlice(statsDMetricBlocklistKey)
	defer config.Datadog.Set(statsDMetricBlocklistKey, originalValues)
	config.Datadog.Set(statsDMetricBlocklistKey, []string{})

	os.Setenv(proxyEnabledEnvVar, "true")
	defer os.Unsetenv(proxyEnabledEnvVar)

	metricAgent := &ServerlessMetricAgent{}
	defer metricAgent.Stop()
	metricAgent.Start(10*time.Second, &MetricConfig{}, &MetricDogStatsD{})

	expected := []string{
		invocationsMetric,
		errorsMetric,
	}

	setValues := config.Datadog.GetStringSlice(statsDMetricBlocklistKey)
	assert.Equal(t, expected, setValues)
}
func TestRaceFlushVersusAddSample(t *testing.T) {

	config.DetectFeatures()

	metricAgent := &ServerlessMetricAgent{}
	defer metricAgent.Stop()
	metricAgent.Start(10*time.Second, &ValidMetricConfigMocked{}, &MetricDogStatsD{})

	assert.NotNil(t, metricAgent.GetMetricChannel())

	go func() {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(10 * time.Millisecond)
		})

		err := http.ListenAndServe("localhost:8888", nil)
		if err != nil {
			panic(err)
		}
	}()

	go func() {
		for i := 0; i < 1000; i++ {
			n := rand.Intn(10)
			time.Sleep(time.Duration(n) * time.Microsecond)
			go SendTimeoutEnhancedMetric([]string{"tag0:value0", "tag1:value1"}, metricAgent.GetMetricChannel())
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
		errorsMetric,
	}
	result := buildMetricBlocklistForProxy(userProvidedBlocklist)
	assert.Equal(t, expected, result)
}

func getAvailablePort(t *testing.T) uint16 {
	conn, err := net.ListenPacket("udp", ":0")
	require.NoError(t, err)
	defer conn.Close()
	return parsePort(t, conn.LocalAddr().String())
}

func parsePort(t *testing.T, addr string) uint16 {
	_, portString, err := net.SplitHostPort(addr)
	require.NoError(t, err)

	port, err := strconv.ParseUint(portString, 10, 16)
	require.NoError(t, err)

	return uint16(port)
}

func TestRaceFlushVersusParsePacket(t *testing.T) {
	port := getAvailablePort(t)
	config.Datadog.SetDefault("dogstatsd_port", port)

	opts := aggregator.DefaultDemultiplexerOptions(nil)
	opts.FlushInterval = 10 * time.Millisecond
	opts.DontStartForwarders = true
	demux := aggregator.InitAndStartAgentDemultiplexer(opts, "hostname")

	metricOut, _, _ := demux.Aggregator().GetBufferedChannels()
	s, err := dogstatsd.NewServer(demux, nil)
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
			select {
			case res := <-metricOut:
				assert.Equal(t, 1, len(res))
				sample := res[0]
				assert.NotNil(t, sample)
			case <-time.After(30 * time.Second):
				finish.Done()
				assert.FailNow(t, "Timeout on receive channel")
			}
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
