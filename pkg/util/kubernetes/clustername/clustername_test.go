// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package clustername

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestGetClusterName(t *testing.T) {
	mockConfig := config.Mock()
	data := newClusterNameData()

	var testClusterName = "laika"
	mockConfig.Set("cluster_name", testClusterName)
	defer mockConfig.Set("cluster_name", nil)

	assert.Equal(t, testClusterName, getClusterName(data))

	// Test caching and reset
	var newClusterName = "youri"
	mockConfig.Set("cluster_name", newClusterName)
	assert.Equal(t, testClusterName, getClusterName(data))
	freshData := newClusterNameData()
	assert.Equal(t, newClusterName, getClusterName(freshData))

	var dotClusterName = "aclusternamewitha.dot"
	mockConfig.Set("cluster_name", dotClusterName)
	data = newClusterNameData()
	assert.Equal(t, dotClusterName, getClusterName(data))

	var dotsClusterName = "a.cluster.name.with.dots"
	mockConfig.Set("cluster_name", dotsClusterName)
	data = newClusterNameData()
	assert.Equal(t, dotsClusterName, getClusterName(data))

	// Test invalid cluster names
	for _, invalidClusterName := range []string{
		"Capital",
		"with_underscore",
		"with_dot._underscore",
		"toolongtoolongtoolongtoolongtoolongtoolong"} {
		mockConfig.Set("cluster_name", invalidClusterName)
		freshData = newClusterNameData()
		assert.Equal(t, "", getClusterName(freshData))
	}
}

func TestGetClusterID(t *testing.T) {
	// missing env
	cid, err := GetClusterID()
	assert.Empty(t, cid)
	assert.NotNil(t, err)

	// too short
	os.Setenv(clusterIDEnv, "foo")
	cid, err = GetClusterID()
	assert.Empty(t, cid)
	assert.NotNil(t, err)

	// too long
	os.Setenv(clusterIDEnv, "d801b2b1-4811-11ea-8618-121d4d0938a44444444")
	cid, err = GetClusterID()
	assert.Empty(t, cid)
	assert.NotNil(t, err)

	// just right
	testID := "d801b2b1-4811-11ea-8618-121d4d0938a3"
	os.Setenv(clusterIDEnv, testID)
	cid, err = GetClusterID()
	assert.Equal(t, testID, cid)
	assert.Nil(t, err)
}
