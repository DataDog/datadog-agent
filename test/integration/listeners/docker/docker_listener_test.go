// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package listeners

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	log "github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/collector/listeners"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/test/integration/utils"
)

type DockerListenerTestSuite struct {
	suite.Suite
	compose    utils.ComposeConf
	listener   listeners.ServiceListener
	dockerutil *docker.DockerUtil
	newSvc     chan listeners.Service
	delSvc     chan listeners.Service
	stop       chan struct{}
	m          sync.RWMutex
}

func (suite *DockerListenerTestSuite) SetupSuite() {
	tagger.Init()

	var err error
	suite.dockerutil, err = docker.GetDockerUtil()
	require.Nil(suite.T(), err, "can't connect to docker")

	suite.compose = utils.ComposeConf{
		ProjectName: "dockerlistener",
		FilePath:    "testdata/redis.yaml",
	}
}

func (suite *DockerListenerTestSuite) SetupTest() {
	dl, err := listeners.NewDockerListener()
	if err != nil {
		panic(err)
	}
	suite.listener = dl

	suite.newSvc = make(chan listeners.Service, 10)
	suite.delSvc = make(chan listeners.Service, 10)
}

func (suite *DockerListenerTestSuite) TearDownTest() {
	suite.listener = nil
	suite.stopContainers()
}

func (suite *DockerListenerTestSuite) startContainers() ([]string, error) {
	output, err := suite.compose.Start()
	if err != nil {
		log.Errorf("error starting containers:\n%s", string(output))
		return nil, err
	}
	return suite.compose.ListContainers()
}

func (suite *DockerListenerTestSuite) stopContainers() error {
	output, err := suite.compose.Stop()
	if err != nil {
		log.Errorf("error stopping containers:\n%s", string(output))
	}
	return err
}

// Listens in a channel until it. If several events are received for the same containerIDs, the last one is returned
func (suite *DockerListenerTestSuite) getServices(containerIDs []string, channel chan listeners.Service, timeout time.Duration) (map[string]listeners.Service, error) {
	services := make(map[string]listeners.Service)
	timeoutTicker := time.NewTicker(timeout)

	for {
		select {
		case svc := <-channel:
			for _, id := range containerIDs {
				if string(svc.GetID()) == id {
					services[id] = svc
					if len(services) == len(containerIDs) {
						return services, nil
					}
				}
			}
			log.Infof("ignoring service from container ID %s", svc.GetID())
		case <-timeoutTicker.C:
			return services, fmt.Errorf("timeout listening for services, only got %d, expecting %d", len(services), len(containerIDs))
		}
	}
}

// Starts the listener AFTER the containers have started
func (suite *DockerListenerTestSuite) TestListenAfterStart() {
	suite.m.RLock()
	defer suite.m.RUnlock()

	containerIDs, err := suite.startContainers()
	assert.Nil(suite.T(), err)
	assert.Len(suite.T(), containerIDs, 2)
	log.Infof("got container IDs %s from compose", containerIDs)

	// Start listening after the containers started, they'll be listed in the init
	suite.listener.Listen(suite.newSvc, suite.delSvc)

	suite.commonSection(containerIDs)
}

// Starts the listener AFTER the containers have started
func (suite *DockerListenerTestSuite) TestListenBeforeStart() {
	suite.m.RLock()
	defer suite.m.RUnlock()

	// Start listening after the containers started, they'll be detected via docker events
	suite.listener.Listen(suite.newSvc, suite.delSvc)

	containerIDs, err := suite.startContainers()
	assert.Nil(suite.T(), err)
	assert.Len(suite.T(), containerIDs, 2)
	log.Infof("got container IDs %s from compose", containerIDs)

	suite.commonSection(containerIDs)
}

// Common section for both scenarios
func (suite *DockerListenerTestSuite) commonSection(containerIDs []string) {
	// We should get 2 new services
	services, err := suite.getServices(containerIDs, suite.newSvc, 5*time.Second)
	assert.Nil(suite.T(), err)
	assert.Len(suite.T(), services, 2)

	expectedIDs := make(map[string][]string)
	for _, container := range containerIDs {
		inspect, err := suite.dockerutil.Inspect(container, false)
		assert.Nil(suite.T(), err)
		if strings.Contains(inspect.Name, "redis-with-id") {
			expectedIDs[container] = []string{"custom-id"}
		} else {
			expectedIDs[container] = []string{"docker://" + container, "redis"}
		}
	}

	for _, service := range services {
		pid, err := service.GetPid()
		assert.Nil(suite.T(), err)
		assert.True(suite.T(), pid > 0)
		hosts, err := service.GetHosts()
		assert.Nil(suite.T(), err)
		assert.Len(suite.T(), hosts, 1)
		ports, err := service.GetPorts()
		assert.Nil(suite.T(), err)
		assert.Len(suite.T(), ports, 1)
		tags, err := service.GetTags()
		assert.Nil(suite.T(), err)

		assert.Contains(suite.T(), tags, "docker_image:redis:latest")
		assert.Contains(suite.T(), tags, "image_name:redis")
		assert.Contains(suite.T(), tags, "image_tag:latest")

		adIDs, err := service.GetADIdentifiers()
		assert.Nil(suite.T(), err)
		assert.Equal(suite.T(), expectedIDs[string(service.GetID())], adIDs)
	}

	suite.stopContainers()

	// We should get 2 stopped services
	services, err = suite.getServices(containerIDs, suite.delSvc, 5*time.Second)
	assert.Nil(suite.T(), err)
	assert.Len(suite.T(), services, 2)
}

func TestDockerListenerSuite(t *testing.T) {
	suite.Run(t, &DockerListenerTestSuite{})
}
