// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package listeners

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/collector/listeners"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
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
	}
}

func (suite *DockerListenerTestSuite) SetupSuite() {
	docker.InitDockerUtil(&docker.Config{
		CacheDuration:  10 * time.Second,
		CollectNetwork: true,
		Whitelist:      config.Datadog.GetStringSlice("ac_include"),
		Blacklist:      config.Datadog.GetStringSlice("ac_exclude"),
	})
	tagger.Init()
	utils.PullImage(suite.redisImage)
}

func (suite *DockerListenerTestSuite) SetupTest() {
	dl, err := listeners.NewDockerListener()
	if err != nil {
		panic(err)
	}

	suite.listener = dl
}

func (suite *DockerListenerTestSuite) TearDownTest() {
	suite.listener = nil
	suite.containerID = ""
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

// this tests init
func (suite *DockerListenerTestSuite) TestListenWithInit() {
	suite.m.RLock()
	defer suite.m.RUnlock()

	suite.containerStart()

	suite.newSvc = make(chan listeners.Service, 1) // buffered channel to avoid blocking
	suite.delSvc = make(chan listeners.Service)

	suite.listener.Listen(suite.newSvc, suite.delSvc)

	// the listener should have posted the new service at this point
	createdSvc := <-suite.newSvc

	services := suite.listener.GetServices()
	assert.Len(suite.T(), services, 1)
	service, found := services[suite.containerID]
	assert.True(suite.T(), found)
	pid, _ := service.GetPid()
	assert.True(suite.T(), pid > 0)
	hosts, _ := service.GetHosts()
	assert.Len(suite.T(), hosts, 1)
	ports, _ := service.GetPorts()
	assert.Len(suite.T(), ports, 1)
	tags, _ := service.GetTags()
	assert.Contains(suite.T(), tags, "docker_image:redis:latest", "image_name:redis", "image_tag:latest", "container_name:datadog-agent-test-redis")
	// The fifth tag should be the container_id
	assert.Len(suite.T(), tags, 5)
	assert.Equal(suite.T(), createdSvc, services[suite.containerID])
	assert.Equal(suite.T(), suite.containerID, createdSvc.GetID())

	suite.containerRemove()

	// the listener should have put the service in the delSvc channel at
	// this point, grab it
	oldSvc := <-suite.delSvc

	services = suite.listener.GetServices()

	assert.Len(suite.T(), services, 0)
	assert.Equal(suite.T(), oldSvc, createdSvc)
}

// this tests processEvent, createService and removeService as well
func (suite *DockerListenerTestSuite) TestListen() {
	suite.m.RLock()
	defer suite.m.RUnlock()

	suite.newSvc = make(chan listeners.Service)
	suite.delSvc = make(chan listeners.Service)

	suite.listener.Listen(suite.newSvc, suite.delSvc)

	suite.containerStart()

	// the listener should have posted the new service at this point
	createdSvc := <-suite.newSvc

	services := suite.listener.GetServices()
	assert.Len(suite.T(), services, 1)
	service, found := services[suite.containerID]
	assert.True(suite.T(), found)
	pid, _ := service.GetPid()
	assert.True(suite.T(), pid > 0)
	hosts, _ := service.GetHosts()
	assert.Len(suite.T(), hosts, 0)
	ports, _ := service.GetPorts()
	assert.Len(suite.T(), ports, 0)
	tags, _ := service.GetTags()
	assert.Contains(suite.T(), tags, "docker_image:redis:latest", "image_name:redis", "image_tag:latest", "container_name:datadog-agent-test-redis")
	// The fifth tag should be the container_id
	assert.Len(suite.T(), tags, 5)
	assert.Equal(suite.T(), createdSvc, services[suite.containerID])
	assert.Equal(suite.T(), suite.containerID, createdSvc.GetID())

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
