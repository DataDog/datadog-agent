// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

func TestScheduleUnschedule(t *testing.T) {
	store := newClusterStore()
	dispatcher := newDispatcher(store)
	stored, err := dispatcher.getAllConfigs()
	assert.NoError(t, err)
	assert.Len(t, stored, 0)

	config1 := integration.Config{
		Name:         "non-cluster-check",
		ClusterCheck: false,
	}
	config2 := integration.Config{
		Name:         "cluster-check",
		ClusterCheck: true,
	}

	dispatcher.Schedule([]integration.Config{config1, config2})
	stored, err = dispatcher.getAllConfigs()
	assert.NoError(t, err)
	assert.Len(t, stored, 1)
	assert.Contains(t, stored, config2)

	dispatcher.Unschedule([]integration.Config{config1, config2})
	stored, err = dispatcher.getAllConfigs()
	assert.NoError(t, err)
	assert.Len(t, stored, 0)
	requireNotLocked(t, store)
}
