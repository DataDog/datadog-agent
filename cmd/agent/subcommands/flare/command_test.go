// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"errors"
	"testing"

	"github.com/hashicorp/go-multierror"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	assert "github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type mockProfileCollector struct {
	mock.Mock
}

func (m *mockProfileCollector) CreatePerformanceProfile(prefix, debugURL string, cpusec int, target *flare.ProfileData) error {
	args := m.Called(prefix, debugURL, cpusec, target)
	return args.Error(0)
}

func TestReadProfileData(t *testing.T) {
	m := &mockProfileCollector{}
	defer m.AssertExpectations(t)

	mockConfig := config.Mock(t)
	mockConfig.Set("expvar_port", "1001")
	mockConfig.Set("apm_config.enabled", true)
	mockConfig.Set("apm_config.receiver_port", "1002")
	mockConfig.Set("apm_config.receiver_timeout", "10")
	mockConfig.Set("process_config.expvar_port", "1003")

	pdata := &flare.ProfileData{}

	m.On("CreatePerformanceProfile", "core", "http://127.0.0.1:1001/debug/pprof", 30, pdata).Return(nil)
	m.On("CreatePerformanceProfile", "trace", "http://127.0.0.1:1002/debug/pprof", 9, pdata).Return(nil)
	m.On("CreatePerformanceProfile", "process", "http://127.0.0.1:1003/debug/pprof", 30, pdata).Return(nil)
	m.On("CreatePerformanceProfile", "security-agent", "http://127.0.0.1:5011/debug/pprof", 30, pdata).Return(nil)

	err := readProfileData(&cliParams{}, pdata, 30, m.CreatePerformanceProfile)
	assert.NoError(t, err)
}

func TestReadProfileDataNoTraceAgent(t *testing.T) {
	m := &mockProfileCollector{}
	defer m.AssertExpectations(t)

	mockConfig := config.Mock(t)
	mockConfig.Set("expvar_port", "1001")
	mockConfig.Set("apm_config.enabled", false)
	mockConfig.Set("process_config.expvar_port", "1003")

	pdata := &flare.ProfileData{}

	m.On("CreatePerformanceProfile", "core", "http://127.0.0.1:1001/debug/pprof", 30, pdata).Return(nil)
	m.On("CreatePerformanceProfile", "process", "http://127.0.0.1:1003/debug/pprof", 30, pdata).Return(nil)
	m.On("CreatePerformanceProfile", "security-agent", "http://127.0.0.1:5011/debug/pprof", 30, pdata).Return(nil)

	err := readProfileData(&cliParams{}, pdata, 30, m.CreatePerformanceProfile)
	assert.NoError(t, err)
}

func TestReadProfileDataErrors(t *testing.T) {
	m := &mockProfileCollector{}
	defer m.AssertExpectations(t)

	mockConfig := config.Mock(t)
	mockConfig.Set("expvar_port", "1001")
	mockConfig.Set("apm_config.enabled", true)
	mockConfig.Set("apm_config.receiver_port", "1002")
	mockConfig.Set("apm_config.receiver_timeout", "10")
	mockConfig.Set("process_config.expvar_port", "1003")

	pdata := &flare.ProfileData{}

	m.On("CreatePerformanceProfile", "core", "http://127.0.0.1:1001/debug/pprof", 30, pdata).Return(errors.New("can't connect to core agent"))
	m.On("CreatePerformanceProfile", "trace", "http://127.0.0.1:1002/debug/pprof", 9, pdata).Return(errors.New("can't connect to trace agent"))
	m.On("CreatePerformanceProfile", "process", "http://127.0.0.1:1003/debug/pprof", 30, pdata).Return(nil)
	m.On("CreatePerformanceProfile", "security-agent", "http://127.0.0.1:5011/debug/pprof", 30, pdata).Return(nil)

	err := readProfileData(&cliParams{}, pdata, 30, m.CreatePerformanceProfile)

	merr, ok := err.(*multierror.Error)
	assert.True(t, ok)
	assert.Len(t, merr.Errors, 2)
	assert.ErrorContains(t, merr.Errors[0], "can't connect to core agent")
	assert.ErrorContains(t, merr.Errors[1], "can't connect to trace agent")
}

func TestCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"flare", "1234"},
		makeFlare,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, []string{"1234"}, cliParams.args)
			require.Equal(t, true, coreParams.ConfigLoadSecrets)
		})
}
