// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer_windows

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

type baseSuite struct {
	e2e.BaseSuite[environments.WindowsHost]
	pipelineID string
}

func (s *baseSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)

	if pipelineID, pipelineIDFound := os.LookupEnv("E2E_PIPELINE_ID"); pipelineIDFound {
		s.pipelineID = pipelineID
	} else {
		s.T().Logf("E2E_PIPELINE_ID env var is not set, this test requires this variable to be set to work")
		s.T().FailNow()
	}
}

func (s *baseSuite) It() *it {
	host := s.Env().RemoteHost
	return &it{t: s.T(), h: host}
}

type it struct {
	t *testing.T
	h *components.RemoteHost
}

type itService struct {
	*it
	serviceConfig *common.ServiceConfig
}

func (it *it) HasAService(serviceName string) *itService {
	serviceConfig, err := common.GetServiceConfig(it.h, serviceName)
	assert.NoError(it.t, err)
	if err != nil {
		it.t.FailNow()
	}
	return &itService{it: it, serviceConfig: serviceConfig}
}

func (it *itService) WithStatus(status string) *itService {
	status, err := common.GetServiceStatus(it.h, it.serviceConfig.ServiceName)
	assert.NoError(it.t, err)
	assert.Equal(it.t, status, status)
	return it
}

func (it *itService) WithLogon(logon string) *itService {
	assert.Equal(it.t, logon, it.serviceConfig.UserName)
	return it
}

func (it *itService) WithUserSid(sid string) *itService {
	assert.Equal(it.t, sid, it.serviceConfig.UserSID)
	return it
}
