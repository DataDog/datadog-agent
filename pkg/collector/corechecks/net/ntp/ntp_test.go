// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ntp

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/beevik/ntp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders"
)

var (
	ntpCfgString = `
offset_threshold: 60
port: 123
version: 3
timeout: 5
`
	ntpCfgStringDatadogPool = `
offset_threshold: 60
port: 123
version: 3
timeout: 5
hosts: [ 0.datadog.pool.ntp.org, 1.datadog.pool.ntp.org, 2.datadog.pool.ntp.org, 3.datadog.pool.ntp.org ]
`
	offset = 10
)

func testNTPQueryError(_ string, _ ntp.QueryOptions) (*ntp.Response, error) {
	return nil, fmt.Errorf("test error from NTP")
}

func testNTPQueryInvalid(_ string, _ ntp.QueryOptions) (*ntp.Response, error) {
	return &ntp.Response{
		ClockOffset: time.Duration(offset) * time.Second,
		Stratum:     20,
	}, nil
}

func testNTPQuery(_ string, _ ntp.QueryOptions) (*ntp.Response, error) {
	return &ntp.Response{
		ClockOffset: time.Duration(offset) * time.Second,
		Stratum:     1,
	}, nil
}

func TestNTPOK(t *testing.T) {
	ntpCfg := []byte(ntpCfgString)
	ntpInitCfg := []byte("")

	offset = 21
	ntpQuery = testNTPQuery
	defer func() { ntpQuery = ntp.QueryWithOptions }()

	ntpCheck := new(NTPCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	ntpCheck.Configure(senderManager, integration.FakeConfigHash, ntpCfg, ntpInitCfg, "test")
	mockSender := mocksender.NewMockSenderWithSenderManager(ntpCheck.ID(), senderManager)

	mockSender.On("Gauge", "ntp.offset", float64(21), "", []string(nil)).Return().Times(1)
	mockSender.On("ServiceCheck",
		"ntp.in_sync",
		servicecheck.ServiceCheckOK,
		"",
		[]string(nil),
		"").Return().Times(1)

	mockSender.On("Commit").Return().Times(1)
	ntpCheck.Run()

	mockSender.AssertExpectations(t)
	mockSender.AssertNumberOfCalls(t, "Gauge", 1)
	mockSender.AssertNumberOfCalls(t, "ServiceCheck", 1)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}

func TestNTPCritical(t *testing.T) {
	ntpCfg := []byte(ntpCfgString)
	ntpInitCfg := []byte("")

	offset = 100
	ntpQuery = testNTPQuery
	defer func() { ntpQuery = ntp.QueryWithOptions }()

	ntpCheck := new(NTPCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	ntpCheck.Configure(senderManager, integration.FakeConfigHash, ntpCfg, ntpInitCfg, "test")

	mockSender := mocksender.NewMockSenderWithSenderManager(ntpCheck.ID(), senderManager)

	mockSender.On("Gauge", "ntp.offset", float64(100), "", []string(nil)).Return().Times(1)
	mockSender.On("ServiceCheck",
		"ntp.in_sync",
		servicecheck.ServiceCheckCritical,
		"",
		[]string(nil),
		"Offset 100 is higher than offset threshold (60 secs)").Return().Times(1)

	mockSender.On("Commit").Return().Times(1)
	ntpCheck.Run()

	mockSender.AssertExpectations(t)
	mockSender.AssertNumberOfCalls(t, "Gauge", 1)
	mockSender.AssertNumberOfCalls(t, "ServiceCheck", 1)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}

func TestNTPError(t *testing.T) {
	ntpCfg := []byte(ntpCfgStringDatadogPool)
	ntpInitCfg := []byte("")

	ntpQuery = testNTPQueryError
	defer func() { ntpQuery = ntp.QueryWithOptions }()

	ntpCheck := new(NTPCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	ntpCheck.Configure(senderManager, integration.FakeConfigHash, ntpCfg, ntpInitCfg, "test")

	mockSender := mocksender.NewMockSenderWithSenderManager(ntpCheck.ID(), senderManager)
	mockSender.On("ServiceCheck",
		"ntp.in_sync",
		servicecheck.ServiceCheckUnknown,
		"",
		[]string(nil),
		mock.AnythingOfType("string")).Return().Times(1)

	mockSender.On("Commit").Return().Times(1)
	err := ntpCheck.Run()

	mockSender.AssertExpectations(t)
	mockSender.AssertNumberOfCalls(t, "Gauge", 0)
	mockSender.AssertNumberOfCalls(t, "ServiceCheck", 1)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
	assert.Error(t, err)
	assert.EqualError(t, err, "failed to get clock offset from any ntp host: [ 0.datadog.pool.ntp.org, 1.datadog.pool.ntp.org, 2.datadog.pool.ntp.org, 3.datadog.pool.ntp.org ]. See https://docs.datadoghq.com/agent/troubleshooting/ntp/ for more details on how to debug this issue")
}

func TestNTPInvalid(t *testing.T) {
	ntpCfg := []byte(ntpCfgString)
	ntpInitCfg := []byte("")

	ntpQuery = testNTPQueryInvalid
	defer func() { ntpQuery = ntp.QueryWithOptions }()

	ntpCheck := new(NTPCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	ntpCheck.Configure(senderManager, integration.FakeConfigHash, ntpCfg, ntpInitCfg, "test")

	mockSender := mocksender.NewMockSenderWithSenderManager(ntpCheck.ID(), senderManager)
	mockSender.On("ServiceCheck",
		"ntp.in_sync",
		servicecheck.ServiceCheckUnknown,
		"",
		[]string(nil),
		mock.AnythingOfType("string")).Return().Times(1)

	mockSender.On("Commit").Return().Times(1)
	err := ntpCheck.Run()

	mockSender.AssertExpectations(t)
	mockSender.AssertNumberOfCalls(t, "Gauge", 0)
	mockSender.AssertNumberOfCalls(t, "ServiceCheck", 1)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
	assert.Error(t, err)
}

func TestNTPNegativeOffsetCritical(t *testing.T) {
	ntpCfg := []byte(ntpCfgString)
	ntpInitCfg := []byte("")

	offset = -100
	ntpQuery = testNTPQuery
	defer func() { ntpQuery = ntp.QueryWithOptions }()

	ntpCheck := new(NTPCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	ntpCheck.Configure(senderManager, integration.FakeConfigHash, ntpCfg, ntpInitCfg, "test")

	mockSender := mocksender.NewMockSenderWithSenderManager(ntpCheck.ID(), senderManager)

	mockSender.On("Gauge", "ntp.offset", float64(-100), "", []string(nil)).Return().Times(1)
	mockSender.On("ServiceCheck",
		"ntp.in_sync",
		servicecheck.ServiceCheckCritical,
		"",
		[]string(nil),
		"Offset -100 is higher than offset threshold (60 secs)").Return().Times(1)

	mockSender.On("Commit").Return().Times(1)
	ntpCheck.Run()

	mockSender.AssertExpectations(t)
	mockSender.AssertNumberOfCalls(t, "Gauge", 1)
	mockSender.AssertNumberOfCalls(t, "ServiceCheck", 1)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}

func TestNTPResiliencyOK(t *testing.T) {
	ntpCfg := []byte(`
hosts:
  - 1
  - 400
  - 2
`)
	ntpInitCfg := []byte("")

	offset = 1
	ntpQuery = func(host string, _ ntp.QueryOptions) (*ntp.Response, error) {
		o, _ := strconv.Atoi(host)
		return &ntp.Response{
			ClockOffset: time.Duration(o) * time.Second,
			Stratum:     15,
		}, nil
	}
	defer func() { ntpQuery = ntp.QueryWithOptions }()

	ntpCheck := new(NTPCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	ntpCheck.Configure(senderManager, integration.FakeConfigHash, ntpCfg, ntpInitCfg, "test")

	mockSender := mocksender.NewMockSenderWithSenderManager(ntpCheck.ID(), senderManager)

	mockSender.On("Gauge", "ntp.offset", float64(2), "", []string(nil)).Return().Times(1)
	mockSender.On("ServiceCheck",
		"ntp.in_sync",
		servicecheck.ServiceCheckOK,
		"",
		[]string(nil),
		"").Return().Times(1)

	mockSender.On("Commit").Return().Times(1)
	ntpCheck.Run()

	mockSender.AssertExpectations(t)
	mockSender.AssertNumberOfCalls(t, "Gauge", 1)
	mockSender.AssertNumberOfCalls(t, "ServiceCheck", 1)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}

func TestNTPResiliencyCritical(t *testing.T) {
	ntpCfg := []byte(`
hosts:
  - 1
  - 400
  - 400
`)
	ntpInitCfg := []byte("")

	offset = 1
	ntpQuery = func(host string, _ ntp.QueryOptions) (*ntp.Response, error) {
		o, _ := strconv.Atoi(host)
		return &ntp.Response{
			ClockOffset: time.Duration(o) * time.Second,
			Stratum:     15,
		}, nil
	}
	defer func() { ntpQuery = ntp.QueryWithOptions }()

	ntpCheck := new(NTPCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	ntpCheck.Configure(senderManager, integration.FakeConfigHash, ntpCfg, ntpInitCfg, "test")

	mockSender := mocksender.NewMockSenderWithSenderManager(ntpCheck.ID(), senderManager)

	mockSender.On("Gauge", "ntp.offset", float64(400), "", []string(nil)).Return().Times(1)
	mockSender.On("ServiceCheck",
		"ntp.in_sync",
		servicecheck.ServiceCheckCritical,
		"",
		[]string(nil),
		"Offset 400 is higher than offset threshold (60 secs)").Return().Times(1)

	mockSender.On("Commit").Return().Times(1)
	ntpCheck.Run()

	mockSender.AssertExpectations(t)
	mockSender.AssertNumberOfCalls(t, "Gauge", 1)
	mockSender.AssertNumberOfCalls(t, "ServiceCheck", 1)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}

func TestHostConfigsMerge(t *testing.T) {
	expectedHosts := []string{"0.time.dogo", "1.time.dogo", "2.time.dogo"}
	testedConfig := []byte(`
host: 0.time.dogo
hosts:
  - 1.time.dogo
  - 2.time.dogo
`)

	ntpCheck := new(NTPCheck)
	ntpCheck.Configure(aggregator.NewNoOpSenderManager(), integration.FakeConfigHash, testedConfig, []byte(""), "test")

	assert.Equal(t, expectedHosts, ntpCheck.cfg.instance.Hosts)
}

func TestHostConfigsMergeNoDuplicate(t *testing.T) {
	expectedHosts := []string{"0.time.dogo", "1.time.dogo", "2.time.dogo"}
	testedConfig := []byte(`
host: 0.time.dogo
hosts:
  - 0.time.dogo
  - 1.time.dogo
  - 2.time.dogo
`)

	ntpCheck := new(NTPCheck)
	ntpCheck.Configure(aggregator.NewNoOpSenderManager(), integration.FakeConfigHash, testedConfig, []byte(""), "test")

	assert.Equal(t, expectedHosts, ntpCheck.cfg.instance.Hosts)
}

func TestHostConfig(t *testing.T) {
	expectedHosts := []string{"time.dogo"}
	testedConfig := []byte(`
host: time.dogo
`)

	ntpCheck := new(NTPCheck)
	ntpCheck.Configure(aggregator.NewNoOpSenderManager(), integration.FakeConfigHash, testedConfig, []byte(""), "test")

	assert.Equal(t, expectedHosts, ntpCheck.cfg.instance.Hosts)
}

func TestHostsConfig(t *testing.T) {
	expectedHosts := []string{"0.time.dogo", "1.time.dogo"}
	testedConfig := []byte(`
hosts:
  - 0.time.dogo
  - 1.time.dogo
`)

	ntpCheck := new(NTPCheck)
	ntpCheck.Configure(aggregator.NewNoOpSenderManager(), integration.FakeConfigHash, testedConfig, []byte(""), "test")

	assert.Equal(t, expectedHosts, ntpCheck.cfg.instance.Hosts)
}

func TestDefaultHostConfig(t *testing.T) {
	// for this test, do not check the cloud providers
	getCloudProviderNTPHosts = func(_ context.Context) []string { return nil }
	defer func() { getCloudProviderNTPHosts = cloudproviders.GetCloudProviderNTPHosts }()

	mockConfig := configmock.New(t)

	expectedHosts := []string{"0.datadog.pool.ntp.org", "1.datadog.pool.ntp.org", "2.datadog.pool.ntp.org", "3.datadog.pool.ntp.org"}
	testedConfig := []byte(``)
	mockConfig.SetWithoutSource("cloud_provider_metadata", []string{})

	ntpCheck := new(NTPCheck)
	ntpCheck.Configure(aggregator.NewNoOpSenderManager(), integration.FakeConfigHash, testedConfig, []byte(""), "test")

	assert.Equal(t, expectedHosts, ntpCheck.cfg.instance.Hosts)
}

func TestNTPPortConfig(t *testing.T) {
	var detectedPorts []int

	ntpQuery = func(host string, opt ntp.QueryOptions) (*ntp.Response, error) {
		detectedPorts = append(detectedPorts, opt.Port)
		return testNTPQuery(host, opt)
	}
	defer func() { ntpQuery = ntp.QueryWithOptions }()

	ntpCheck := new(NTPCheck)
	const expectedPort = 42
	ntpCfg := []byte(fmt.Sprintf(`
offset_threshold: 60
port: %d
`, expectedPort))
	err := ntpCheck.Configure(aggregator.NewNoOpSenderManager(), integration.FakeConfigHash, ntpCfg, []byte(""), "test")
	assert.Nil(t, err)

	mockSender := mocksender.NewMockSender(ntpCheck.ID())
	mockSender.SetupAcceptAll()

	ntpCheck.Run()

	for _, port := range detectedPorts {
		assert.Equal(t, expectedPort, port)
	}
}

func TestNTPPortNotInt(t *testing.T) {
	ntpCheck := new(NTPCheck)
	ntpCfg := []byte(`
offset_threshold: 60
port: ntp`)

	err := ntpCheck.Configure(aggregator.NewNoOpSenderManager(), integration.FakeConfigHash, ntpCfg, []byte(""), "test")
	assert.EqualError(t, err, "yaml: unmarshal errors:\n  line 3: cannot unmarshal !!str `ntp` into int")
}

func TestNTPUseLocalDefinedServers(t *testing.T) {
	const localNtpServerTest = "local NTP server"
	getLocalServers := func() ([]string, error) { return []string{localNtpServerTest}, nil }

	configUseLocalServer := ntpConfig{}
	err := configUseLocalServer.parse([]byte("use_local_defined_servers: true"), nil, getLocalServers)
	assert.NoError(t, err)
	assert.True(t, configUseLocalServer.instance.UseLocalDefinedServers)
	assert.Equal(t, []string{localNtpServerTest}, configUseLocalServer.instance.Hosts)

	defaultConfig := ntpConfig{}
	err = defaultConfig.parse([]byte("use_local_defined_servers: false"), nil, getLocalServers)
	assert.NoError(t, err)
	assert.False(t, defaultConfig.instance.UseLocalDefinedServers)
	assert.NotEqual(t, configUseLocalServer.instance.Hosts, defaultConfig.instance.Hosts)
}

func TestNTPDynamicServerRediscovery(t *testing.T) {
	// Test that servers are re-discovered on each Run() call when UseLocalDefinedServers is true

	// Mock the getLocalDefinedNTPServers function to return different servers over time
	currentServers := []string{"initial-server.com"}
	originalGetLocalServers := getLocalDefinedNTPServersFunc
	getLocalDefinedNTPServersFunc = func() ([]string, error) {
		return currentServers, nil
	}
	defer func() { getLocalDefinedNTPServersFunc = originalGetLocalServers }()

	// Mock NTP query to avoid actual network calls
	ntpQuery = testNTPQuery
	defer func() { ntpQuery = ntp.QueryWithOptions }()

	// Configure check with UseLocalDefinedServers enabled
	ntpCfg := []byte("use_local_defined_servers: true")
	ntpCheck := new(NTPCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	err := ntpCheck.Configure(senderManager, integration.FakeConfigHash, ntpCfg, []byte(""), "test")
	assert.NoError(t, err)

	// Verify initial configuration
	assert.Equal(t, []string{"initial-server.com"}, ntpCheck.cfg.instance.Hosts)

	// Create mock sender
	mockSender := mocksender.NewMockSenderWithSenderManager(ntpCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	// First run - should use initial servers
	err = ntpCheck.Run()
	assert.NoError(t, err)
	assert.Equal(t, []string{"initial-server.com"}, ntpCheck.cfg.instance.Hosts)

	// Change the servers returned by the discovery function (simulating DC promotion)
	currentServers = []string{"new-pdc.com"}

	// Second run - should discover and use new servers
	err = ntpCheck.Run()
	assert.NoError(t, err)
	assert.Equal(t, []string{"new-pdc.com"}, ntpCheck.cfg.instance.Hosts)

	// Third run with same servers - should keep the same servers
	err = ntpCheck.Run()
	assert.NoError(t, err)
	assert.Equal(t, []string{"new-pdc.com"}, ntpCheck.cfg.instance.Hosts)
}
