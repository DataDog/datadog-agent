// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package listeners

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	log "github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
	acTelemetry "github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	compcfg "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/catalog"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/test/integration/utils"
)

type DockerListenerTestSuite struct {
	suite.Suite
	compose        utils.ComposeConf
	listener       listeners.ServiceListener
	dockerutil     *docker.DockerUtil
	newSvc         chan listeners.Service
	delSvc         chan listeners.Service
	stop           chan struct{}
	m              sync.RWMutex
	wmeta          workloadmeta.Component
	telemetryStore *acTelemetry.Store
	tagger         tagger.Component
}

type deps struct {
	fx.In
	Tagger    tagger.Component
	WMeta     workloadmeta.Component
	Telemetry telemetry.Component
}

func (suite *DockerListenerTestSuite) SetupSuite() {
	containers.ResetSharedFilter()

	pkglogsetup.SetupLogger(
		pkglogsetup.LoggerName("test"),
		"debug",
		"",
		"",
		false,
		true,
		false,
		pkgconfigsetup.Datadog(),
	)

	overrides := map[string]interface{}{
		"ac_include": []string{"name:.*redis.*"},
		"ac_exclude": []string{"image:datadog/docker-library:redis.*"},
	}

	var err error
	deps := fxutil.Test[deps](suite.T(), fx.Options(
		core.MockBundle(),
		fx.Replace(compcfg.MockParams{
			Overrides: overrides,
		}),
		wmcatalog.GetCatalog(),
		workloadmetafx.Module(workloadmeta.NewParams()),
		taggerimpl.Module(),
		fx.Supply(tagger.NewTaggerParams()),
	))
	env.SetFeatures(suite.T(), env.Docker)
	suite.wmeta = deps.WMeta
	suite.telemetryStore = acTelemetry.NewStore(deps.Telemetry)
	suite.dockerutil, err = docker.GetDockerUtil()
	suite.tagger = deps.Tagger
	require.Nil(suite.T(), err, "can't connect to docker")

	suite.compose = utils.ComposeConf{
		ProjectName: "dockerlistener",
		FilePath:    "testdata/redis.yaml",
	}
}

func (suite *DockerListenerTestSuite) TearDownSuite() {
	containers.ResetSharedFilter()
}

func (suite *DockerListenerTestSuite) SetupTest() {
	dl, err := listeners.NewContainerListener(listeners.ServiceListernerDeps{
		Config:    &pkgconfigsetup.Listeners{},
		Wmeta:     optional.NewOption(suite.wmeta),
		Telemetry: suite.telemetryStore,
		Tagger:    suite.tagger,
	})
	if err != nil {
		panic(err)
	}
	suite.listener = dl

	suite.newSvc = make(chan listeners.Service, 10)
	suite.delSvc = make(chan listeners.Service, 10)
}

func (suite *DockerListenerTestSuite) TearDownTest() {
	suite.listener.Stop()
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

// Listens in a channel until it receives one service per listed container.
// If several events are received for the same containerIDs, the last one is returned
func (suite *DockerListenerTestSuite) getServices(targetIDs, excludedIDs []string, channel chan listeners.Service, timeout time.Duration) (map[string]listeners.Service, error) {
	services := make(map[string]listeners.Service)
	timeoutTicker := time.NewTicker(timeout)

	for {
		select {
		case svc := <-channel:
			for _, id := range targetIDs {
				if strings.HasSuffix(svc.GetServiceID(), id) {
					log.Infof("Service matches container %s, keeping", id)
					services[id] = svc
					log.Infof("Got services for %d containers so far, out of %d wanted", len(services), len(targetIDs))
					if len(services) == len(targetIDs) {
						log.Infof("Got all %d services, returning", len(services))
						return services, nil
					}
				}
			}
			for _, id := range excludedIDs {
				if strings.HasSuffix(svc.GetServiceID(), id) {
					return services, fmt.Errorf("got service for excluded container %s", id)
				}
			}
		case <-timeoutTicker.C:
			return services, fmt.Errorf("timeout listening for services, only got %d, expecting %d", len(services), len(targetIDs))
		}
	}
}

// Starts the listener AFTER the containers have started
func (suite *DockerListenerTestSuite) TestListenAfterStart() {
	suite.m.RLock()
	defer suite.m.RUnlock()

	containerIDs, err := suite.startContainers()
	assert.Nil(suite.T(), err)
	assert.Len(suite.T(), containerIDs, 3)
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
	assert.Len(suite.T(), containerIDs, 3)
	log.Infof("got container IDs %s from compose", containerIDs)

	suite.commonSection(containerIDs)
}

// Common section for both scenarios
func (suite *DockerListenerTestSuite) commonSection(containerIDs []string) {
	ctx := context.Background()
	expectedADIDs := make(map[string][]string)
	var includedIDs, excludedIDs []string
	var excludedEntity string

	for _, container := range containerIDs {
		inspect, err := suite.dockerutil.Inspect(ctx, container, false)
		assert.Nil(suite.T(), err)
		entity := fmt.Sprintf("docker://%s", container)
		if strings.Contains(inspect.Name, "excluded") {
			excludedEntity = containers.BuildEntityName(string(workloadmeta.ContainerRuntimeDocker), container)
			excludedIDs = append(excludedIDs, container)
			continue
		}
		includedIDs = append(includedIDs, container)
		if strings.Contains(inspect.Name, "redis-with-id") {
			expectedADIDs[entity] = []string{"custom-id"}
		} else {
			expectedADIDs[entity] = []string{
				entity,
				"datadog/docker-library",
				"docker-library",
			}
		}
	}

	// We should get 2 new services
	services, err := suite.getServices(includedIDs, excludedIDs, suite.newSvc, 5*time.Second)
	assert.Nil(suite.T(), err)
	assert.Len(suite.T(), services, 2)

	for _, service := range services {
		pid, err := service.GetPid(ctx)
		assert.Nil(suite.T(), err)
		assert.True(suite.T(), pid > 0)
		hosts, err := service.GetHosts(ctx)
		assert.Nil(suite.T(), err)
		assert.Len(suite.T(), hosts, 1)
		ports, err := service.GetPorts(ctx)
		assert.Nil(suite.T(), err)
		assert.Len(suite.T(), ports, 1)

		entity := service.GetServiceID()
		expectedTags, found := expectedADIDs[entity]
		assert.True(suite.T(), found, "entity not found in expected ones")

		tags, err := service.GetTags()
		assert.Nil(suite.T(), err)
		assert.Contains(suite.T(), tags, "docker_image:datadog/docker-library:redis_3_2_11-alpine")
		assert.Contains(suite.T(), tags, "image_name:datadog/docker-library")
		assert.Contains(suite.T(), tags, "image_tag:redis_3_2_11-alpine")

		adIDs, err := service.GetADIdentifiers(ctx)
		assert.Nil(suite.T(), err)
		assert.Equal(suite.T(), expectedTags, adIDs)
	}

	// Listen for late messages
	select {
	case svc := <-suite.newSvc:
		if svc.GetServiceID() == excludedEntity {
			assert.FailNowf(suite.T(), "received service for excluded container %s", excludedEntity)
		}
	case <-time.After(250 * time.Millisecond):
		// all good
	}

	suite.stopContainers()

	// We should get 2 stopped services
	services, err = suite.getServices(containerIDs, excludedIDs, suite.delSvc, 5*time.Second)
	assert.Error(suite.T(), err)
	assert.Len(suite.T(), services, 2)

	// Listen for late messages
	select {
	case svc := <-suite.delSvc:
		if svc.GetServiceID() == excludedEntity {
			assert.FailNowf(suite.T(), "received service for excluded container %s", excludedEntity)
		}
	case <-time.After(250 * time.Millisecond):
		// all good
	}
}

func TestDockerListenerSuite(t *testing.T) {
	suite.Run(t, &DockerListenerTestSuite{})
}
