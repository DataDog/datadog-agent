// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker
// +build kubeapiserver

package kubernetes

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ericchiang/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func TestKubeEvents(t *testing.T) {
	// Start compose stack
	compose, err := initAPIServerCompose()
	require.Nil(t, err)
	output, err := compose.Start()
	defer compose.Stop()
	require.Nil(t, err, string(output))

	// Init apiclient
	pwd, err := os.Getwd()
	require.Nil(t, err)
	kubeConfigPath := filepath.Join(pwd, "testdata", "kubeconfig.json")
	config.Datadog.Set("kubernetes_kubeconfig_path", kubeConfigPath)
	apiclient, err := apiserver.GetAPIClient()
	require.Nil(t, err)

	// Init own client to write the events
	var config *k8s.Config
	config, err = apiserver.ParseKubeConfig(kubeConfigPath)
	require.Nil(t, err)
	rawclient, err := k8s.NewClient(config)
	require.Nil(t, err)
	core := rawclient.CoreV1()
	require.NotNil(t, core)

	// Ignore potential startup events
	_, _, initresversion, err := apiclient.LatestEvents("0")
	require.Nil(t, err)

	// Create started event
	testReference := createObjectReference("default", "integration_test", "event_test")
	startedEvent := createEvent("default", "test_started", "started", testReference)
	_, err = core.CreateEvent(context.Background(), startedEvent)
	require.Nil(t, err)

	// Test we get the new started event
	added, modified, resversion, err := apiclient.LatestEvents(initresversion)
	require.Nil(t, err)
	assert.Len(t, added, 1)
	assert.Len(t, modified, 0)
	assert.Equal(t, "started", *added[0].Reason)

	// Create tick event
	tickEvent := createEvent("default", "test_tick", "tick", testReference)
	_, err = core.CreateEvent(context.Background(), tickEvent)
	require.Nil(t, err)

	// Test we get the new tick event
	added, modified, resversion, err = apiclient.LatestEvents(resversion)
	require.Nil(t, err)
	assert.Len(t, added, 1)
	assert.Len(t, modified, 0)
	assert.Equal(t, "tick", *added[0].Reason)

	// Update tick event
	pointer2 := int32(2)
	tickEvent2 := added[0]
	tickEvent2.Count = &pointer2
	tickEvent3, err := core.UpdateEvent(context.Background(), tickEvent2)
	require.Nil(t, err)

	// Update tick event a second time
	pointer3 := int32(3)
	tickEvent3.Count = &pointer3
	_, err = core.UpdateEvent(context.Background(), tickEvent3)
	require.Nil(t, err)

	// Test we get the two modified test events
	added, modified, resversion, err = apiclient.LatestEvents(resversion)
	require.Nil(t, err)
	assert.Len(t, added, 0)
	assert.Len(t, modified, 2)
	assert.Equal(t, "tick", *modified[0].Reason)
	assert.EqualValues(t, 2, *modified[0].Count)
	assert.Equal(t, "tick", *modified[1].Reason)
	assert.EqualValues(t, 3, *modified[1].Count)
	assert.EqualValues(t, *modified[0].Metadata.Uid, *modified[1].Metadata.Uid)

	// We should get nothing new now
	added, modified, resversion, err = apiclient.LatestEvents(resversion)
	require.Nil(t, err)
	assert.Len(t, added, 0)
	assert.Len(t, modified, 0)

	// We should get 2+0 events from initresversion
	// apiserver does not send updates to objects if the add is in the same bucket
	added, modified, resversion, err = apiclient.LatestEvents(initresversion)
	require.Nil(t, err)
	assert.Len(t, added, 2)
	assert.Len(t, modified, 0)
}
