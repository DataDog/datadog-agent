// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker
// +build kubeapiserver

package kubernetes

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	log "github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
)

const (
	setupTimeout     = 10 * time.Second
	eventReadTimeout = 500 * time.Millisecond
)

type testSuite struct {
	suite.Suite
	apiClient      *apiserver.APIClient
	kubeConfigPath string
}

func TestSuiteKube(t *testing.T) {
	mockConfig := config.NewMock()
	s := &testSuite{}

	// Start compose stack
	compose, err := initAPIServerCompose()
	require.Nil(t, err)
	output, err := compose.Start()
	defer compose.Stop()
	require.Nil(t, err, string(output))

	// Init apiclient
	pwd, err := os.Getwd()
	require.Nil(t, err)
	s.kubeConfigPath = filepath.Join(pwd, "testdata", "kubeconfig.json")
	mockConfig.Set("kubernetes_kubeconfig_path", s.kubeConfigPath)
	_, err = os.Stat(s.kubeConfigPath)
	require.Nil(t, err, fmt.Sprintf("%v", err))

	suite.Run(t, s)
}

func (suite *testSuite) SetupTest() {
	var err error

	tick := time.NewTicker(time.Millisecond * 500)
	timeout := time.NewTicker(setupTimeout)
	for {
		select {
		case <-timeout.C:
			require.FailNow(suite.T(), "timeout after %s", setupTimeout.String())

		case <-tick.C:
			suite.apiClient, err = apiserver.GetAPIClient()
			if err != nil {
				log.Debugf("cannot init: %s", err)
				continue
			}
			// Confirm that we can query the kube-apiserver's resources
			log.Debugf("trying to get LatestEvents")
			_, _, resV, err := suite.apiClient.LatestEvents("0", eventReadTimeout)
			if err == nil {
				log.Debugf("successfully get LatestEvents: %s", resV)
				return
			}
			log.Debugf("cannot get LatestEvents: %s", err)
		}
	}
}

func (suite *testSuite) TestKubeEvents() {
	mockConfig := config.NewMock()

	// Init own client to write the events
	mockConfig.Set("kubernetes_kubeconfig_path", suite.kubeConfigPath)
	c, err := apiserver.GetAPIClient()

	require.NoError(suite.T(), err)

	core := c.Cl.CoreV1()
	require.NotNil(suite.T(), core)

	// Ignore potential startup events
	_, _, initresversion, err := suite.apiClient.LatestEvents("0", eventReadTimeout)
	require.NoError(suite.T(), err)

	// Create started event
	testReference := createObjectReference("default", "integration_test", "event_test")
	startedEvent := createEvent("default", "test_started", "started", *testReference)
	_, err = core.Events("default").Create(startedEvent)
	require.NoError(suite.T(), err)

	// Test we get the new started event
	added, modified, resversion, err := suite.apiClient.LatestEvents(initresversion, eventReadTimeout)
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), added, 1)
	assert.Len(suite.T(), modified, 0)
	assert.Equal(suite.T(), "started", added[0].Reason)

	// Create tick event
	tickEvent := createEvent("default", "test_tick", "tick", *testReference)
	_, err = core.Events("default").Create(tickEvent)
	require.NoError(suite.T(), err)

	// Test we get the new tick event
	added, modified, resversion, err = suite.apiClient.LatestEvents(resversion, eventReadTimeout)
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), added, 1)
	assert.Len(suite.T(), modified, 0)
	assert.Equal(suite.T(), "tick", added[0].Reason)

	// Update tick event
	pointer2 := int32(2)
	tickEvent2 := added[0]
	tickEvent2.Count = pointer2
	tickEvent3, err := core.Events("default").Update(tickEvent2)
	require.NoError(suite.T(), err)

	// Update tick event a second time
	pointer3 := int32(3)
	tickEvent3.Count = pointer3
	_, err = core.Events("default").Update(tickEvent3)
	require.NoError(suite.T(), err)

	// Test we get the two modified test events
	added, modified, resversion, err = suite.apiClient.LatestEvents(resversion, eventReadTimeout)
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), added, 0)
	assert.Len(suite.T(), modified, 2)
	assert.Equal(suite.T(), "tick", modified[0].Reason)
	assert.EqualValues(suite.T(), 2, modified[0].Count)
	assert.Equal(suite.T(), "tick", modified[1].Reason)
	assert.EqualValues(suite.T(), 3, modified[1].Count)
	assert.EqualValues(suite.T(), modified[0].InvolvedObject.UID, modified[1].InvolvedObject.UID)

	// We should get nothing new now
	added, modified, resversion, err = suite.apiClient.LatestEvents(resversion, eventReadTimeout)
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), added, 0)
	assert.Len(suite.T(), modified, 0)

	// We should get 2+0 events from initresversion
	// apiserver does not send updates to objects if the add is in the same bucket
	added, modified, _, err = suite.apiClient.LatestEvents(initresversion, eventReadTimeout)
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), added, 2)
	assert.Len(suite.T(), modified, 0)
}

func (suite *testSuite) TestHostnameProvider() {
	mockConfig := config.NewMock()

	// Init own client to write the events
	mockConfig.Set("kubernetes_kubeconfig_path", suite.kubeConfigPath)
	c, err := apiserver.GetAPIClient()

	require.NoError(suite.T(), err)

	core := c.Cl.CoreV1()
	require.NotNil(suite.T(), core)

	// Create a dummy pod
	myHostname, err := os.Hostname()
	require.NoError(suite.T(), err)
	dummyPod := createPodOnNode("default", myHostname, "target.host")

	// Register it in the apiserver
	_, err = core.Pods("default").Create(dummyPod)
	require.NoError(suite.T(), err)
	defer core.Pods("default").Delete(myHostname, nil)

	// Hostname provider should return the expected value
	foundHost, err := apiserver.HostnameProvider("")
	assert.Equal(suite.T(), "target.host", foundHost)

	// Testing hostname when a cluster name is set
	var testClusterName = "Laika"
	mockConfig.Set("cluster_name", testClusterName)
	clustername.ResetClusterName()
	defer mockConfig.Set("cluster_name", "")
	defer clustername.ResetClusterName()

	foundHost, err = apiserver.HostnameProvider("")
	assert.Equal(suite.T(), "target.host-Laika", foundHost)
}
