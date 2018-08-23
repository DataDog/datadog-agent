// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package clustername

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestGetClusterName(t *testing.T) {
	var testClusterName = "Laika"
	config.Datadog.Set("cluster_name", testClusterName)
	defer config.Datadog.Set("cluster_name", nil)

	assert.Equal(t, testClusterName, GetClusterName())
	assert.Equal(t, initDone, true)
}
