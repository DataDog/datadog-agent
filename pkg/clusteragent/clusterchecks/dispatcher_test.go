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
	requireNotLocked(t, store)

	config1 := integration.Config{
		Name:         "non-cluster-check",
		ClusterCheck: false,
	}
	config2 := integration.Config{
		Name:         "cluster-check",
		ClusterCheck: true,
	}

	dispatcher.Schedule([]integration.Config{config1, config2})
	requireNotLocked(t, store)

	registered := store.getAllConfigs()
	assert.Len(t, registered, 1)
	assert.Contains(t, registered, config2)
	requireNotLocked(t, store)

	dispatcher.Unschedule([]integration.Config{config1, config2})
	requireNotLocked(t, store)

	assert.Len(t, store.getAllConfigs(), 0)
	requireNotLocked(t, store)
}
