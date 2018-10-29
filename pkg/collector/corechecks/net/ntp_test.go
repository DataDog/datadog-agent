// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

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
port: ntp
version: 3
timeout: 5
`
	offset = 10
)

func testNTPQueryError(host string, version int) (*ntp.Response, error) {
	return nil, fmt.Errorf("test error from NTP")
}

func testNTPQuery(host string, version int) (*ntp.Response, error) {
	return &ntp.Response{
		ClockOffset: time.Duration(offset) * time.Second,
	}, nil
}

func TestNTPOK(t *testing.T) {
	var ntpCfg = []byte(ntpCfgString)
	var ntpInitCfg = []byte("")

	offset = 21
	ntpQuery = testNTPQuery
	defer func() { ntpQuery = ntp.Query }()

	ntpCheck := new(NTPCheck)
	ntpCheck.Configure(ntpCfg, ntpInitCfg)

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
	defer func() { ntpQuery = ntp.Query }()

	ntpCheck := new(NTPCheck)
	ntpCheck.Configure(ntpCfg, ntpInitCfg)

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
	defer func() { ntpQuery = ntp.Query }()

	ntpCheck := new(NTPCheck)
	ntpCheck.Configure(ntpCfg, ntpInitCfg)

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
	defer func() { ntpQuery = ntp.Query }()

	ntpCheck := new(NTPCheck)
	ntpCheck.Configure(ntpCfg, ntpInitCfg)

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
	ntpQuery = func(host string, version int) (*ntp.Response, error) {
		o, _ := strconv.Atoi(host)
		return &ntp.Response{
			ClockOffset: time.Duration(o) * time.Second,
		}, nil
	}
	defer func() { ntpQuery = ntp.Query }()

	ntpCheck := new(NTPCheck)
	ntpCheck.Configure(ntpCfg, ntpInitCfg)

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
	ntpQuery = func(host string, version int) (*ntp.Response, error) {
		o, _ := strconv.Atoi(host)
		return &ntp.Response{
			ClockOffset: time.Duration(o) * time.Second,
		}, nil
	}
	defer func() { ntpQuery = ntp.Query }()

	ntpCheck := new(NTPCheck)
	ntpCheck.Configure(ntpCfg, ntpInitCfg)

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
	ntpCheck.Configure(testedConfig, []byte(""))

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
	ntpCheck.Configure(testedConfig, []byte(""))

	assert.Equal(t, expectedHosts, ntpCheck.cfg.instance.Hosts)
}

func TestHostConfig(t *testing.T) {
	expectedHosts := []string{"time.dogo"}
	testedConfig := []byte(`
host: time.dogo
`)

	ntpCheck := new(NTPCheck)
	ntpCheck.Configure(testedConfig, []byte(""))

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
	ntpCheck.Configure(testedConfig, []byte(""))

	assert.Equal(t, expectedHosts, ntpCheck.cfg.instance.Hosts)
}

func TestDefaultHostConfig(t *testing.T) {
	expectedHosts := []string{"0.datadog.pool.ntp.org", "1.datadog.pool.ntp.org", "2.datadog.pool.ntp.org", "3.datadog.pool.ntp.org"}
	testedConfig := []byte(``)

	ntpCheck := new(NTPCheck)
	ntpCheck.Configure(testedConfig, []byte(""))

	assert.Equal(t, expectedHosts, ntpCheck.cfg.instance.Hosts)
}
