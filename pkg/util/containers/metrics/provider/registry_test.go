// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package provider

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/stretchr/testify/assert"
)

func TestCollectorRegistry(t *testing.T) {
	c := newCollectorRegistry()

	assert.Nil(t, c.effectiveCollectors[RuntimeMetadata{runtime: RuntimeNameDocker}])

	// Check for collectors (none are registered, should not change output)
	c.retryCollectors(nil, optional.NewNoneOption[workloadmeta.Component]())
	assert.Nil(t, c.effectiveCollectors[RuntimeMetadata{runtime: RuntimeNameDocker}])

	c.registerCollector(
		CollectorFactory{
			ID: "dummy1",
			Constructor: func(*Cache, optional.Option[workloadmeta.Component]) (CollectorMetadata, error) {
				collector := dummyCollector{
					id:              "dummy1",
					selfContainerID: "dummy1",
				}

				return collector.constructor(10,
					RuntimeMetadata{runtime: RuntimeNameDocker},
					RuntimeMetadata{runtime: RuntimeNameContainerd},
					RuntimeMetadata{runtime: RuntimeNameCRIO},
				)
			},
		},
	)

	c.registerCollector(
		CollectorFactory{
			ID: "dummy2",
			Constructor: func(*Cache, optional.Option[workloadmeta.Component]) (CollectorMetadata, error) {
				return CollectorMetadata{}, ErrPermaFail
			},
		},
	)

	var dummy3Retries int
	c.registerCollector(
		CollectorFactory{
			ID: "dummy3",
			Constructor: func(*Cache, optional.Option[workloadmeta.Component]) (CollectorMetadata, error) {
				if dummy3Retries < 2 {
					dummy3Retries++
					return CollectorMetadata{}, fmt.Errorf("not yet okay")
				}

				collector := dummyCollector{
					id:              "dummy3",
					selfContainerID: "dummy3",
				}

				return collector.constructor(9, RuntimeMetadata{runtime: RuntimeNameCRIO})
			},
		},
	)

	// No retry, should still fail
	assert.Nil(t, c.effectiveCollectors[RuntimeMetadata{runtime: RuntimeNameDocker}])

	// dummy1 should answer to everything
	assertCollectors := func(expected map[RuntimeMetadata]string) {
		actual := make(map[RuntimeMetadata]string)

		for _, runtime := range []RuntimeMetadata{
			{runtime: RuntimeNameDocker},
			{runtime: RuntimeNameContainerd},
			{runtime: RuntimeNameCRIO},
		} {
			val, err := c.effectiveCollectors[runtime].SelfContainerID.Collector.GetSelfContainerID()
			assert.NoError(t, err)
			actual[runtime] = val
		}

		assert.Equal(t, expected, actual)
	}

	wmeta := optional.NewNoneOption[workloadmeta.Component]()
	collectorsToRetry := c.retryCollectors(nil, wmeta)
	assert.Equal(t, 1, collectorsToRetry)
	assertCollectors(map[RuntimeMetadata]string{
		{runtime: RuntimeNameDocker}:     "dummy1",
		{runtime: RuntimeNameContainerd}: "dummy1",
		{runtime: RuntimeNameCRIO}:       "dummy1",
	})

	// dummy3 still not there, dummy2 never ok
	collectorsToRetry = c.retryCollectors(nil, wmeta)
	assert.Equal(t, 1, collectorsToRetry)
	assertCollectors(map[RuntimeMetadata]string{
		{runtime: RuntimeNameDocker}:     "dummy1",
		{runtime: RuntimeNameContainerd}: "dummy1",
		{runtime: RuntimeNameCRIO}:       "dummy1",
	})

	// dummy3 should pop up
	collectorsToRetry = c.retryCollectors(nil, wmeta)
	assert.Equal(t, 0, collectorsToRetry)
	assertCollectors(map[RuntimeMetadata]string{
		{runtime: RuntimeNameDocker}:     "dummy1",
		{runtime: RuntimeNameContainerd}: "dummy1",
		{runtime: RuntimeNameCRIO}:       "dummy3",
	})

	// Registering a new collector
	c.registerCollector(
		CollectorFactory{
			ID: "dummy4",
			Constructor: func(*Cache, optional.Option[workloadmeta.Component]) (CollectorMetadata, error) {
				collector := dummyCollector{
					id:              "dummy4",
					selfContainerID: "dummy4",
				}

				return collector.constructor(8, RuntimeMetadata{runtime: RuntimeNameDocker})
			},
		},
	)

	collectorsToRetry = c.retryCollectors(nil, wmeta)
	assert.Equal(t, 0, collectorsToRetry)
	assertCollectors(map[RuntimeMetadata]string{
		{runtime: RuntimeNameDocker}:     "dummy4",
		{runtime: RuntimeNameContainerd}: "dummy1",
		{runtime: RuntimeNameCRIO}:       "dummy3",
	})
}
