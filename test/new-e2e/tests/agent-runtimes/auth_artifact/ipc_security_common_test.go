// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package auth

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"regexp"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	osComp "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	svcmanager "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common/svc-manager"
)

// iteration is the number of time we will check the auth stack
const iteration = 100

// timeout is the time we will wait for one iteration to success
const timeout = time.Minute

const lockSuffix = ".lock"
const authLoadedRegex = `successfully loaded the IPC auth primitives \(fingerprint: ([\d\w]{16})\)`
const authCreatedRegex = "successfully created artifact"

type pattern struct {
	regex      *regexp.Regexp
	isRequired bool
}

type authArtifactBase struct {
	e2e.BaseSuite[environments.Host]
	sync.Mutex
	patterns      []pattern
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

	// Compile regexp pattern
	signaturePattern, err := regexp.Compile(authLoadedRegex)
	a.Require().NoError(err)
	creattionPattern, err := regexp.Compile(authCreatedRegex)
	a.Require().NoError(err)

	a.patterns = []pattern{
		{signaturePattern, true},
		{creattionPattern, false},
	}

	for i := 0; i < iteration; i++ {
		a.checkAuthStack()
	}
}

func (a *authArtifactBase) checkAuthStack() {
	// Starting Agent processes
	a.T().Log("starting agent service")
	_, err := a.svcManager.Start(a.svcName)
	a.Require().NoError(err)

	authSigns := make([]string, len(a.agentProcesses))
	g := new(errgroup.Group)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for i, agentName := range a.agentProcesses {
		g.Go(func() error {
			result, err := a.checkAgentLogs(ctx, agentName)
			if err != nil {
				return fmt.Errorf("error while checking logs of %s: %v", agentName, err)
			}

			authSigns[i] = result
			return nil
		})
	}

	err = g.Wait()
	a.Require().NoError(err)
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

func (a *authArtifactBase) checkAgentLogs(ctx context.Context, agentName string) (string, error) {
	logLocation := a.pathJoinFunction(a.logFolder, agentName+".log")

	// Trying to get the log file
	var stdout io.Reader
	var err error

	isReady := a.Eventually(func() bool {
		a.Lock()
		_, _, stdout, err = a.Env().RemoteHost.Start(fmt.Sprintf(a.readLogCmdTmpl, logLocation))
		a.Unlock()
		return err == nil
	}, 10*time.Second, 1*time.Second)

	if !isReady {
		return "", fmt.Errorf("unable to get log file %v: %v", logLocation, err)
	}

	a.T().Logf("starting reading from %v", agentName)

	scanner := bufio.NewScanner(stdout)
	linechan := make(chan string)
	errchan := make(chan error)

	go func() {
		for scanner.Scan() {
			linechan <- scanner.Text()
		}
		errchan <- scanner.Err()
	}()

Loop:
	for {
		select {
		case line := <-linechan:
			for _, pattern := range a.patterns {
				if found := pattern.regex.FindString(line); found != "" {
					a.T().Logf("found pattern: %s", line)
					if pattern.isRequired {
						return found, nil
					}
				}
			}

		case err = <-errchan:
			err = fmt.Errorf("unable to find pattern in %s: %v", agentName, err)
			break Loop

		case <-ctx.Done():
			err = fmt.Errorf("timeout while reading log file %v", logLocation)
			break Loop
		}
	}
	return "", err
}
