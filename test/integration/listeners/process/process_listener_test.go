// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listeners

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	log "github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/tagger"
)

type ProcessListenerTestSuite struct {
	suite.Suite
	listener      listeners.ServiceListener
	newSvc        chan listeners.Service
	delSvc        chan listeners.Service
	mockProcesses map[int]mockProcess
	stop          chan struct{}
	m             sync.RWMutex
}

type mockProcess struct {
	proc *os.Process
	name string
	cmd  string
}

func (suite *ProcessListenerTestSuite) SetupSuite() {
	tagger.Init()

	config.SetupLogger(
		"debug",
		"",
		"",
		false,
		true,
		false,
	)
	// Custom process polling interval for tests
	listeners.ProcessPollInterval = 3 * time.Second
	suite.mockProcesses = map[int]mockProcess{}
}

func (suite *ProcessListenerTestSuite) SetupTest() {
	pl, err := listeners.NewProcessListener()
	if err != nil {
		panic(err)
	}
	suite.listener = pl

	suite.newSvc = make(chan listeners.Service, 30)
	suite.delSvc = make(chan listeners.Service, 30)
}

func (suite *ProcessListenerTestSuite) TearDownTest() {
	suite.listener.Stop()
	suite.listener = nil
	suite.mockProcesses = map[int]mockProcess{}
}

func (suite *ProcessListenerTestSuite) startProcesses() error {
	// mocking go scripts
	scripts := []mockProcess{
		{cmd: "./testdata/redis-server/redis-server", name: "redisdb"},
		{cmd: "./testdata/consul-agent/consul-agent", name: "consul"},
		{cmd: "./testdata/memcached/memcached", name: "mcache"},
	}

	goCmd, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("Couldn't find golang binary in path")
	}

	for _, script := range scripts {
		buildCmd := exec.Command(goCmd, "build", "-o", script.cmd, fmt.Sprintf("%s.go", script.cmd))
		if err := buildCmd.Start(); err != nil {
			return fmt.Errorf("Couldn't build script %v: %s", script.cmd, err)
		}
		if err := buildCmd.Wait(); err != nil {
			return fmt.Errorf("Couldn't wait the end of the build for script %v: %s", script.cmd, err)
		}

		cmd := exec.Command(script.cmd)

		if err := cmd.Start(); err != nil {
			return fmt.Errorf("Couldn't start process %v: %s", script.cmd, err)
		}

		script.proc = cmd.Process
		suite.mockProcesses[cmd.Process.Pid] = script
	}

	// Wait for the processes to init their connection
	time.Sleep(3 * time.Second)

	return nil
}

func (suite *ProcessListenerTestSuite) stopProcesses() {
	for pid := range suite.mockProcesses {
		if err := suite.mockProcesses[pid].proc.Kill(); err != nil {
			log.Errorf("Couldn't stop process %s: %s", pid, err)
		}
		suite.mockProcesses[pid].proc.Wait()
	}
}

// Listens in a channel until it receives one service per listed process.
// If several events are received for the same pid, the last one is returned
func (suite *ProcessListenerTestSuite) getServices(channel chan listeners.Service, timeout time.Duration) (map[int]listeners.Service, error) {
	services := make(map[int]listeners.Service)
	timeoutTicker := time.NewTicker(timeout)

	for {
		select {
		case svc := <-channel:
			for pid := range suite.mockProcesses {
				if strings.HasPrefix(svc.GetEntity(), fmt.Sprintf("%d", pid)) {
					log.Infof("Service matches process %s (pid: %d), keeping", svc.GetEntity(), pid)
					services[pid] = svc
					log.Infof("Got services for %d processes so far, out of %d wanted", len(services), len(suite.mockProcesses))
					if len(services) == len(suite.mockProcesses) {
						log.Infof("Got all %d services, returning", len(services))
						return services, nil
					}
				}
			}
		case <-timeoutTicker.C:
			return services, fmt.Errorf("timeout listening for services, only got %d, expecting %d", len(services), len(suite.mockProcesses))
		}
	}
}

// Starts the listener AFTER the process have started
func (suite *ProcessListenerTestSuite) TestListenAfterStart() {
	suite.m.RLock()
	defer suite.m.RUnlock()

	err := suite.startProcesses()
	assert.Nil(suite.T(), err)

	suite.listener.Listen(suite.newSvc, suite.delSvc)
	suite.commonSection(integration.Before)
}

// Starts the listener AFTER the process have started
func (suite *ProcessListenerTestSuite) TestListenBeforeStart() {
	suite.m.RLock()
	defer suite.m.RUnlock()

	suite.listener.Listen(suite.newSvc, suite.delSvc)

	err := suite.startProcesses()
	assert.Nil(suite.T(), err)

	suite.commonSection(integration.After)
}

// Common section for both scenarios
func (suite *ProcessListenerTestSuite) commonSection(ct integration.CreationTime) {
	services, err := suite.getServices(suite.newSvc, 10*time.Second)
	assert.Nil(suite.T(), err)

	for _, service := range services {
		pid, err := service.GetPid()
		assert.Nil(suite.T(), err)
		assert.True(suite.T(), pid > 0)

		hosts, err := service.GetHosts()
		assert.Nil(suite.T(), err)
		assert.Len(suite.T(), hosts, 1)

		ports, err := service.GetPorts()
		assert.Nil(suite.T(), err)

		sockets, err := service.GetUnixSockets()
		assert.Nil(suite.T(), err)

		// Each fake service have either a port or a socket
		assert.Equal(suite.T(), len(ports)+len(sockets), 1)

		creationTime := service.GetCreationTime()
		assert.Equal(suite.T(), creationTime, ct)

		assert.Contains(suite.T(), suite.mockProcesses, pid)
		mocked := suite.mockProcesses[pid]
		entity := service.GetEntity()
		assert.Equal(suite.T(), fmt.Sprintf("%d:%s", pid, mocked.name), entity)

		tags, err := service.GetTags()
		assert.Nil(suite.T(), err)
		assert.Len(suite.T(), tags, 0)

		adIDs, err := service.GetADIdentifiers()
		assert.Nil(suite.T(), err)
		assert.Contains(suite.T(), adIDs, mocked.name)
		assert.Contains(suite.T(), adIDs, mocked.cmd)
	}

	suite.stopProcesses()

	// We should get 3 stopped services
	services, err = suite.getServices(suite.delSvc, 10*time.Second)
	assert.Nil(suite.T(), err)
	assert.Len(suite.T(), services, len(suite.mockProcesses))

}

func TestProcessListenerSuite(t *testing.T) {
	suite.Run(t, &ProcessListenerTestSuite{})
}
