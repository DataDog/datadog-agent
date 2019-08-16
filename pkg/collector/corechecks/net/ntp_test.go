// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package net

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/beevik/ntp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

var (
	ntpCfgString = `
offset_threshold: 60
port: 123
version: 3
timeout: 5
`
	offset = 10
)

func testNTPQueryError(host string, opt ntp.QueryOptions) (*ntp.Response, error) {
	return nil, fmt.Errorf("test error from NTP")
}

func testNTPQueryInvalid(host string, opt ntp.QueryOptions) (*ntp.Response, error) {
	return &ntp.Response{
		ClockOffset: time.Duration(offset) * time.Second,
		Stratum:     20,
	}, nil
}

func testNTPQuery(host string, opt ntp.QueryOptions) (*ntp.Response, error) {
	return &ntp.Response{
		ClockOffset: time.Duration(offset) * time.Second,
		Stratum:     1,
	}, nil
}

func TestNTPOK(t *testing.T) {
	var ntpCfg = []byte(ntpCfgString)
	var ntpInitCfg = []byte("")

	offset = 21
	ntpQuery = testNTPQuery
	defer func() { ntpQuery = ntp.QueryWithOptions }()

	ntpCheck := new(NTPCheck)
	ntpCheck.Configure(ntpCfg, ntpInitCfg, "test")

	mockSender := mocksender.NewMockSender(ntpCheck.ID())

	mockSender.On("Gauge", "ntp.offset", float64(21), "", []string(nil)).Return().Times(1)
	mockSender.On("ServiceCheck",
		"ntp.in_sync",
		metrics.ServiceCheckOK,
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
	var ntpCfg = []byte(ntpCfgString)
	var ntpInitCfg = []byte("")

	offset = 100
	ntpQuery = testNTPQuery
	defer func() { ntpQuery = ntp.QueryWithOptions }()

	ntpCheck := new(NTPCheck)
	ntpCheck.Configure(ntpCfg, ntpInitCfg, "test")

	mockSender := mocksender.NewMockSender(ntpCheck.ID())

	mockSender.On("Gauge", "ntp.offset", float64(100), "", []string(nil)).Return().Times(1)
	mockSender.On("ServiceCheck",
		"ntp.in_sync",
		metrics.ServiceCheckCritical,
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
	var ntpCfg = []byte(ntpCfgString)
	var ntpInitCfg = []byte("")

	ntpQuery = testNTPQueryError
	defer func() { ntpQuery = ntp.QueryWithOptions }()

	ntpCheck := new(NTPCheck)
	ntpCheck.Configure(ntpCfg, ntpInitCfg, "test")

	mockSender := mocksender.NewMockSender(ntpCheck.ID())

	mockSender.On("ServiceCheck",
		"ntp.in_sync",
		metrics.ServiceCheckUnknown,
		"",
		[]string(nil),
		mock.AnythingOfType("string")).Return().Times(1)

	mockSender.On("Commit").Return().Times(1)
	ntpCheck.Run()

	mockSender.AssertExpectations(t)
	mockSender.AssertNumberOfCalls(t, "Gauge", 0)
	mockSender.AssertNumberOfCalls(t, "ServiceCheck", 1)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}

func TestNTPInvalid(t *testing.T) {
	var ntpCfg = []byte(ntpCfgString)
	var ntpInitCfg = []byte("")

	ntpQuery = testNTPQueryInvalid
	defer func() { ntpQuery = ntp.QueryWithOptions }()

	ntpCheck := new(NTPCheck)
	ntpCheck.Configure(ntpCfg, ntpInitCfg, "test")

	mockSender := mocksender.NewMockSender(ntpCheck.ID())

	mockSender.On("ServiceCheck",
		"ntp.in_sync",
		metrics.ServiceCheckUnknown,
		"",
		[]string(nil),
		mock.AnythingOfType("string")).Return().Times(1)

	mockSender.On("Commit").Return().Times(1)
	ntpCheck.Run()

	mockSender.AssertExpectations(t)
	mockSender.AssertNumberOfCalls(t, "Gauge", 0)
	mockSender.AssertNumberOfCalls(t, "ServiceCheck", 1)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}

func TestNTPNegativeOffsetCritical(t *testing.T) {
	var ntpCfg = []byte(ntpCfgString)
	var ntpInitCfg = []byte("")

	offset = -100
	ntpQuery = testNTPQuery
	defer func() { ntpQuery = ntp.QueryWithOptions }()

	ntpCheck := new(NTPCheck)
	ntpCheck.Configure(ntpCfg, ntpInitCfg, "test")

	mockSender := mocksender.NewMockSender(ntpCheck.ID())

	mockSender.On("Gauge", "ntp.offset", float64(-100), "", []string(nil)).Return().Times(1)
	mockSender.On("ServiceCheck",
		"ntp.in_sync",
		metrics.ServiceCheckCritical,
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
	var ntpCfg = []byte(`
hosts:
  - 1
  - 400
  - 2
`)
	var ntpInitCfg = []byte("")

	offset = 1
	ntpQuery = func(host string, opt ntp.QueryOptions) (*ntp.Response, error) {
		o, _ := strconv.Atoi(host)
		return &ntp.Response{
			ClockOffset: time.Duration(o) * time.Second,
			Stratum:     15,
		}, nil
	}
	defer func() { ntpQuery = ntp.QueryWithOptions }()

	ntpCheck := new(NTPCheck)
	ntpCheck.Configure(ntpCfg, ntpInitCfg, "test")

	mockSender := mocksender.NewMockSender(ntpCheck.ID())

	mockSender.On("Gauge", "ntp.offset", float64(2), "", []string(nil)).Return().Times(1)
	mockSender.On("ServiceCheck",
		"ntp.in_sync",
		metrics.ServiceCheckOK,
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
	var ntpCfg = []byte(`
hosts:
  - 1
  - 400
  - 400
`)
	var ntpInitCfg = []byte("")

	offset = 1
	ntpQuery = func(host string, opt ntp.QueryOptions) (*ntp.Response, error) {
		o, _ := strconv.Atoi(host)
		return &ntp.Response{
			ClockOffset: time.Duration(o) * time.Second,
			Stratum:     15,
		}, nil
	}
	defer func() { ntpQuery = ntp.QueryWithOptions }()

	ntpCheck := new(NTPCheck)
	ntpCheck.Configure(ntpCfg, ntpInitCfg, "test")

	mockSender := mocksender.NewMockSender(ntpCheck.ID())

	mockSender.On("Gauge", "ntp.offset", float64(400), "", []string(nil)).Return().Times(1)
	mockSender.On("ServiceCheck",
		"ntp.in_sync",
		metrics.ServiceCheckCritical,
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
	ntpCheck.Configure(testedConfig, []byte(""), "test")

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
	ntpCheck.Configure(testedConfig, []byte(""), "test")

	assert.Equal(t, expectedHosts, ntpCheck.cfg.instance.Hosts)
}

func TestHostConfig(t *testing.T) {
	expectedHosts := []string{"time.dogo"}
	testedConfig := []byte(`
host: time.dogo
`)

	ntpCheck := new(NTPCheck)
	ntpCheck.Configure(testedConfig, []byte(""), "test")

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
	ntpCheck.Configure(testedConfig, []byte(""), "test")

	assert.Equal(t, expectedHosts, ntpCheck.cfg.instance.Hosts)
}

func TestDefaultHostConfig(t *testing.T) {
	expectedHosts := []string{"0.datadog.pool.ntp.org", "1.datadog.pool.ntp.org", "2.datadog.pool.ntp.org", "3.datadog.pool.ntp.org"}
	testedConfig := []byte(``)

	ntpCheck := new(NTPCheck)
	ntpCheck.Configure(testedConfig, []byte(""), "test")

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
	err := ntpCheck.Configure(ntpCfg, []byte(""), "test")
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

	err := ntpCheck.Configure(ntpCfg, []byte(""), "test")
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
