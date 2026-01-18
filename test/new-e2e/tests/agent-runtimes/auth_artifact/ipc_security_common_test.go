// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package auth

import (
	_ "embed"
	"fmt"
	"regexp"
	"sync"
	"time"

	osComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	svcmanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/svc-manager"
)

// timeout is the time we will wait for one iteration to success
const timeout = time.Minute

const lockSuffix = ".lock"

var requiredPattern = regexp.MustCompile(`successfully loaded the IPC auth primitives \(fingerprint: ([\d\w]{16})\)`)
var extraPatterns = []*regexp.Regexp{regexp.MustCompile("successfully created artifact")}

//go:embed fixtures/config.yaml
var agentConfig string

type authArtifactBase struct {
	e2e.BaseSuite[environments.Host]
	sync.Mutex
	svcManager    svcmanager.ServiceManager
	logFolder     string
	authTokenPath string
	ipcCertPath   string
	// Per os specific
	readLogCmdTmpl     string
	removeFilesCmdTmpl string
	pathJoinFunction   func(...string) string
	svcName            string
	agentProcesses     []string
}

func (a *authArtifactBase) TestServersideIPCCertUsage() {
	// checking agent working correctly
	if a.Env().RemoteHost.OSFamily == osComp.WindowsFamily {
		a.svcManager = svcmanager.NewWindows(a.Env().RemoteHost)
	} else {
		a.svcManager = common.GetServiceManager(a.Env().RemoteHost)
	}
	a.Require().NotNil(a.svcManager)

	// Waiting until all Agent load or create auth artifacts
	var err error
	a.logFolder, err = a.Env().RemoteHost.GetLogsFolder()
	a.Require().NoError(err)

	a.checkAuthStack()
}

func (a *authArtifactBase) checkAuthStack() {
	// Starting Agent processes
	a.T().Log("starting agent service")
	_, err := a.svcManager.Start(a.svcName)
	a.Require().NoError(err)

	authSigns := make([]string, len(a.agentProcesses))

	var wg sync.WaitGroup

	for i, agentName := range a.agentProcesses {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := a.checkAgentLogs(agentName)
			authSigns[i] = result
		}()
	}

	wg.Wait()
	a.T().Log("all process initialized their auth stack")

	// checking that signatures are all equals
	a.T().Log("checking that every process have the same hash")
	for i := 1; i < len(a.agentProcesses); i++ {
		a.Require().Equal(authSigns[i-1], authSigns[i])
	}

	// Checking that there is no lock files
	a.T().Log("checking that auth artifacts exists and lock files have been cleaned")
	exist, err := a.Env().RemoteHost.FileExists(a.authTokenPath)
	a.Require().NoError(err)
	a.Require().True(exist)

	exist, err = a.Env().RemoteHost.FileExists(a.authTokenPath + lockSuffix)
	a.Require().NoError(err)
	a.Require().False(exist)

	exist, err = a.Env().RemoteHost.FileExists(a.ipcCertPath)
	a.Require().NoError(err)
	a.Require().True(exist)

	exist, err = a.Env().RemoteHost.FileExists(a.ipcCertPath + lockSuffix)
	a.Require().NoError(err)
	a.Require().False(exist)

	// stoping agent service
	a.T().Log("stopping agent service")
	_, err = a.svcManager.Stop(a.svcName)
	a.Require().NoError(err)

	a.EventuallyWithT(func(c *assert.CollectT) {
		_, err = a.svcManager.Status(a.svcName)
		require.Error(c, err)
	}, 10*time.Second, 1*time.Second, "datadog Agent should be stopped")

	// Removing log files and artifacts files
	a.Env().RemoteHost.MustExecute(fmt.Sprintf(a.removeFilesCmdTmpl, a.logFolder, a.authTokenPath, a.ipcCertPath))
}

func (a *authArtifactBase) checkAgentLogs(agentName string) string {
	logLocation := a.pathJoinFunction(a.logFolder, agentName+".log")

	var result string
	a.EventuallyWithT(func(t *assert.CollectT) {
		content, err := a.Env().RemoteHost.ReadFilePrivileged(logLocation)
		require.NoError(t, err)

		for _, p := range extraPatterns {
			if found := p.Find(content); len(found) > 0 {
				a.T().Logf("found \"%s\" in %s", found, logLocation)
			}
		}

		if found := requiredPattern.Find(content); len(found) > 0 {
			a.T().Logf("found \"%s\" in %s", found, logLocation)
			result = string(found)
		}

		require.NotEmpty(t, result, "no required pattern found in %s", logLocation)
	}, timeout, 1*time.Second, "didn't found \"%s\" in %s", requiredPattern.String(), logLocation)
	return result
}
