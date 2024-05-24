// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package servicediscovery

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
)

type linuxTestSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestLinuxTestSuite(t *testing.T) {
	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awshost.Provisioner()),
	}

	devModeEnv, _ := os.LookupEnv("E2E_DEVMODE")
	if devMode, err := strconv.ParseBool(devModeEnv); err == nil && devMode {
		options = append(options, e2e.WithDevMode())
	}

	e2e.Run(t, &linuxTestSuite{}, options...)
}

func (s *linuxTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	s.provisionServer()
	s.startServices()
}

func (s *linuxTestSuite) TestServiceDiscoveryCheck() {
	t := s.T()

	assert.EventuallyWithT(t, func(t *assert.CollectT) {
		assertRunningChecks(t, s.Env().RemoteHost, []string{"service_discovery"})
	}, 2*time.Minute, 5*time.Second)
}

type checkStatus struct {
	CheckID           string `json:"CheckID"`
	CheckName         string `json:"CheckName"`
	CheckConfigSource string `json:"CheckConfigSource"`
	ExecutionTimes    []int  `json:"ExecutionTimes"`
}

type runnerStats struct {
	Checks map[string]checkStatus `json:"Checks"`
}

type collectorStatus struct {
	RunnerStats runnerStats `json:"runnerStats"`
}

// assertRunningChecks asserts that the given process agent checks are running on the given VM
func assertRunningChecks(t *assert.CollectT, remoteHost *components.RemoteHost, checks []string) {
	statusOutput := remoteHost.MustExecute("sudo datadog-agent status collector --json")

	var status collectorStatus
	err := json.Unmarshal([]byte(statusOutput), &status)
	require.NoError(t, err, "failed to unmarshal agent status")

	for _, c := range checks {
		assert.Contains(t, status.RunnerStats.Checks, c)
	}
}

func (s *linuxTestSuite) provisionServer() {
	err := s.Env().RemoteHost.CopyFolder("testdata", "/home/ubuntu/e2e-test")
	require.NoError(s.T(), err)
	s.Env().RemoteHost.MustExecute("sudo bash /home/ubuntu/e2e-test/provision.sh")
}

func (s *linuxTestSuite) startServices() {
	s.commandWithEnv("PORT=8080 node /home/ubuntu/e2e-test/node/server.js &")
	s.commandWithEnv("PORT=8081 go run /home/ubuntu/e2e-test/go/main.go &")
	s.commandWithEnv("PORT=8082 python3 /home/ubuntu/e2e-test/python/server.py &")
}

func (s *linuxTestSuite) commandWithEnv(command string) {
	s.Env().RemoteHost.MustExecute("source $HOME/.nvm/nvm.sh && " + command)
}
