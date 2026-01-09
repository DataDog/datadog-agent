// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"errors"
	"net"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	delegatedauthmock "github.com/DataDog/datadog-agent/comp/core/delegatedauth/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/listeners"
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
	pkgconfigsetup.LoadDatadog(mockConfig, secretsmock.New(t), delegatedauthmock.New(t), nil)
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
func (m *MetricDogStatsDMocked) NewServer(_ aggregator.Demultiplexer) (dogstatsdServer.ServerlessDogstatsd, error) {
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

func TestRaceFlushVersusParsePacket(t *testing.T) {
	mockConfig := configmock.New(t)
	pkgconfigsetup.LoadDatadog(mockConfig, secretsmock.New(t), delegatedauthmock.New(t), nil)
	mockConfig.SetDefault("dogstatsd_port", listeners.RandomPortName)

	demux, err := aggregator.InitAndStartServerlessDemultiplexer(nil, time.Second*1000, nooptagger.NewComponent(), false)
	require.NoError(t, err, "cannot start Demultiplexer")

	s, err := dogstatsdServer.NewServerlessServer(demux)
	require.NoError(t, err, "cannot start DSD")
	defer s.Stop()

	url := s.UDPLocalAddr()
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
