// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
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
	if os.Getenv("CI") == "true" && runtime.GOOS == "darwin" {
		t.Skip("TestStartDoesNotBlock is known to fail on the macOS Gitlab runners because of the already running Agent")
	}
	mockConfig := configmock.New(t)
	pkgconfigsetup.LoadDatadog(mockConfig, secretsmock.New(t), nil)
	metricAgent := &ServerlessMetricAgent{
		SketchesBucketOffset: time.Second * 10,
		Tagger:               nooptagger.NewComponent(),
	}
	defer metricAgent.Stop()
	metricAgent.Start(10*time.Second, &MetricConfig{}, &MetricDogStatsD{}, false)
}

type InvalidMetricConfigMocked struct{}

func (m *InvalidMetricConfigMocked) GetMultipleEndpoints() (utils.EndpointDescriptorSet, error) {
	return nil, errors.New("error")
}

func TestStartInvalidConfig(t *testing.T) {
	metricAgent := &ServerlessMetricAgent{
		SketchesBucketOffset: time.Second * 10,
		Tagger:               nooptagger.NewComponent(),
	}
	defer metricAgent.Stop()
	metricAgent.Start(1*time.Second, &InvalidMetricConfigMocked{}, &MetricDogStatsD{}, false)
	assert.False(t, metricAgent.IsReady())
}

//nolint:revive // TODO(SERV) Fix revive linter
type MetricDogStatsDMocked struct{}

//nolint:revive // TODO(SERV) Fix revive linter
func (m *MetricDogStatsDMocked) NewServer(_ aggregator.Demultiplexer, _ tagger.Component) (dogstatsdServer.ServerlessDogstatsd, error) {
	return nil, errors.New("error")
}

func TestStartInvalidDogStatsD(t *testing.T) {
	metricAgent := &ServerlessMetricAgent{
		SketchesBucketOffset: time.Second * 10,
		Tagger:               nooptagger.NewComponent(),
	}
	defer metricAgent.Stop()
	metricAgent.Start(1*time.Second, &MetricConfig{}, &MetricDogStatsDMocked{}, false)
	assert.False(t, metricAgent.IsReady())
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
	mockConfig := configmock.New(t)
	port, err := getAvailableUDPPort()
	require.NoError(t, err)
	mockConfig.SetDefault("dogstatsd_port", port)

	demux, err := aggregator.InitAndStartServerlessDemultiplexer(nil, time.Second*1000, nooptagger.NewComponent(), false)
	require.NoError(t, err, "cannot start Demultiplexer")

	s, err := dogstatsdServer.NewServerlessServer(demux, nooptagger.NewComponent())
	require.NoError(t, err, "cannot start DSD")
	defer s.Stop()

	url := fmt.Sprintf("127.0.0.1:%d", mockConfig.GetInt("dogstatsd_port"))
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
		wg.Done()
	}(finish)

	go func(wg *sync.WaitGroup) {
		for i := 0; i < 1000; i++ {
			s.ServerlessFlush(time.Second * 10)
		}
		wg.Done()
	}(finish)

	finish.Wait()
}
