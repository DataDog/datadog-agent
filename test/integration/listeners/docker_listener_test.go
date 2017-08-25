// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package listeners

import (
	"context"
	"sync"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/collector/listeners"
	"github.com/DataDog/datadog-agent/test/integration/utils"
)

type DockerListenerTestSuite struct {
	suite.Suite
	containerName string
	redisImage    string
	containerID   listeners.ID
	listener      *listeners.DockerListener
	newSvc        chan listeners.Service
	delSvc        chan listeners.Service
	stop          chan struct{}
	m             sync.RWMutex
}

// use a constructor to make the suite parametric
func NewDockerListenerTestSuite(redisVersion, containerName string) *DockerListenerTestSuite {
	return &DockerListenerTestSuite{
		containerName: containerName,
		redisImage:    "redis:" + redisVersion,
		stop:          make(chan struct{}, 1),
	}
}

func (suite *DockerListenerTestSuite) SetupSuite() {
	utils.PullImage(suite.redisImage)
}

func (suite *DockerListenerTestSuite) SetupTest() {
	suite.m.Lock()
	defer suite.m.Unlock()

	suite.newSvc = make(chan listeners.Service)
	suite.delSvc = make(chan listeners.Service)

	dl, err := listeners.NewDockerListener(suite.newSvc, suite.delSvc)
	if err != nil {
		panic(err)
	}

	suite.listener = dl
}

func (suite *DockerListenerTestSuite) TearDownTest() {
	suite.listener = nil
	suite.containerID = ""
	suite.stop <- struct{}{}
}

func (suite *DockerListenerTestSuite) containerStart() {
	id, err := utils.StartRedisContainer(suite.redisImage, suite.containerName)
	if err != nil {
		panic(err)
	}
	suite.containerID = listeners.ID(id)
}

func (suite *DockerListenerTestSuite) containerRemove() {
	cli, err := client.NewEnvClient()
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	cli.ContainerRemove(ctx, suite.containerName, types.ContainerRemoveOptions{Force: true})
}

func (suite *DockerListenerTestSuite) TestInit() {
	// Init sends to the newSvc channel, grab the messages so we don't
	// block the test
	go func() {
		suite.m.RLock()
		defer suite.m.RUnlock()

		for {
			select {
			case <-suite.stop:
				return
			case <-suite.newSvc:
			}
		}
	}()

	suite.listener.Init()
	services := suite.listener.GetServices()
	// services might contain other, unrelated containers running on the host,
	// we specifically search for our redis container
	assert.NotContains(suite.T(), services, suite.containerID)

	suite.containerStart()

	suite.listener.Init()
	services = suite.listener.GetServices()
	service, found := services[suite.containerID]
	assert.True(suite.T(), found)
	assert.Len(suite.T(), service.Hosts, 1)
	assert.Len(suite.T(), service.Tags, 0)

	suite.containerRemove()
}

// this tests processEvent, createService and removeService as well
func (suite *DockerListenerTestSuite) TestListen() {
	suite.m.RLock()
	defer suite.m.RUnlock()

	suite.listener.Listen()

	suite.containerStart()

	// the listener should have posted the new service at this point
	createdSvc := <-suite.newSvc

	services := suite.listener.GetServices()
	assert.Len(suite.T(), services, 1)
	assert.Equal(suite.T(), createdSvc, services[suite.containerID])
	assert.Equal(suite.T(), suite.containerID, createdSvc.ID)

	suite.containerRemove()

	// the listener should have put the service in the delSvc channel at
	// this point, grab it
	oldSvc := <-suite.delSvc

	services = suite.listener.GetServices()

	assert.Len(suite.T(), services, 0)
	assert.Equal(suite.T(), oldSvc, createdSvc)
}

func TestDockerListenerSuite(t *testing.T) {
	suite.Run(t, NewDockerListenerTestSuite("latest", "datadog-agent-test-redis"))
}
