// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package provider

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetaCollector(t *testing.T) {
	actualCollector1 := &dummyCollector{
		id: "foo1",
		cIDForPID: map[int]string{
			1: "foo1",
		},
		selfContainerID: "agent1",
		cIDForPodCont: map[string]string{
			"pc-pod1/foo":   "cID1",
			"pc-pod1/i-foo": "cID2",
		},
	}
	actualCollector2 := &dummyCollector{
		id: "foo2",
		cIDForPID: map[int]string{
			2: "foo2",
		},
		selfContainerID: "agent2",
		cIDForPodCont: map[string]string{
			"pc-pod1/foo":   "cID3",
			"pc-pod1/i-foo": "cID4",
		},
	}
	actualCollector3 := &dummyCollector{
		id: "foo3",
		cIDForPID: map[int]string{
			3: "",
		},
		err: errors.New("FailingCollector"),
	}

	metaCollector := newMetaCollector()
	metaCollector.collectorsUpdatedCallback(CollectorCatalog{
		RuntimeMetadata{runtime: RuntimeNameContainerd}: &Collectors{
			ContainerIDForPID: CollectorRef[ContainerIDForPIDRetriever]{
				Collector: actualCollector1,
				Priority:  0,
			},
			SelfContainerID: CollectorRef[SelfContainerIDRetriever]{
				Collector: actualCollector1,
				Priority:  0,
			},
			ContainerIDForPodUIDAndContName: CollectorRef[ContainerIDForPodUIDAndContNameRetriever]{
				Collector: actualCollector1,
			},
		},
		RuntimeMetadata{runtime: RuntimeNameDocker}: &Collectors{
			ContainerIDForPID: CollectorRef[ContainerIDForPIDRetriever]{
				Collector: actualCollector2,
				Priority:  1,
			},
			SelfContainerID: CollectorRef[SelfContainerIDRetriever]{
				Collector: actualCollector2,
				Priority:  1,
			},
			ContainerIDForPodUIDAndContName: CollectorRef[ContainerIDForPodUIDAndContNameRetriever]{
				Collector: actualCollector2,
				Priority:  1,
			},
		},
	})

	cID1, err := metaCollector.GetContainerIDForPID(1, 0)
	assert.NoError(t, err)
	assert.Equal(t, "foo1", cID1)

	cID2, err := metaCollector.GetContainerIDForPID(2, 0)
	assert.NoError(t, err)
	assert.Equal(t, "foo2", cID2)

	cID3, err := metaCollector.GetContainerIDForPID(3, 0)
	assert.NoError(t, err)
	assert.Equal(t, "", cID3)

	cIDPodUIDAndContName, err := metaCollector.ContainerIDForPodUIDAndContName("pod1", "foo", false, 0)
	assert.NoError(t, err)
	assert.Equal(t, "cID1", cIDPodUIDAndContName)

	cIDPodUIDAndContNameInit, err := metaCollector.ContainerIDForPodUIDAndContName("pod1", "foo", true, 0)
	assert.NoError(t, err)
	assert.Equal(t, "cID2", cIDPodUIDAndContNameInit)

	cIDPodUID, err := metaCollector.ContainerIDForPodUIDAndContName("pod1", "", false, 0)
	assert.NoError(t, err)
	assert.Equal(t, "", cIDPodUID)

	cIDContName, err := metaCollector.ContainerIDForPodUIDAndContName("", "foo", false, 0)
	assert.NoError(t, err)
	assert.Equal(t, "", cIDContName)

	cIDEmpty, err := metaCollector.ContainerIDForPodUIDAndContName("", "", false, 0)
	assert.NoError(t, err)
	assert.Equal(t, "", cIDEmpty)

	cIDEmptyInit, err := metaCollector.ContainerIDForPodUIDAndContName("", "", true, 0)
	assert.NoError(t, err)
	assert.Equal(t, "", cIDEmptyInit)

	// Add the failing collector
	metaCollector.collectorsUpdatedCallback(
		CollectorCatalog{
			RuntimeMetadata{runtime: RuntimeNameContainerd}: &Collectors{
				ContainerIDForPID: CollectorRef[ContainerIDForPIDRetriever]{
					Collector: actualCollector1,
					Priority:  0,
				},
				SelfContainerID: CollectorRef[SelfContainerIDRetriever]{
					Collector: actualCollector1,
					Priority:  0,
				},
				ContainerIDForPodUIDAndContName: CollectorRef[ContainerIDForPodUIDAndContNameRetriever]{
					Collector: actualCollector1,
					Priority:  0,
				},
			},
			RuntimeMetadata{runtime: RuntimeNameDocker}: &Collectors{
				ContainerIDForPID: CollectorRef[ContainerIDForPIDRetriever]{
					Collector: actualCollector2,
					Priority:  1,
				},
				SelfContainerID: CollectorRef[SelfContainerIDRetriever]{
					Collector: actualCollector2,
					Priority:  1,
				},
				ContainerIDForPodUIDAndContName: CollectorRef[ContainerIDForPodUIDAndContNameRetriever]{
					Collector: actualCollector2,
					Priority:  1,
				},
			},
			RuntimeMetadata{runtime: RuntimeNameCRIO}: &Collectors{
				ContainerIDForPID: CollectorRef[ContainerIDForPIDRetriever]{
					Collector: actualCollector3,
					Priority:  2,
				},
				SelfContainerID: CollectorRef[SelfContainerIDRetriever]{
					Collector: actualCollector3,
					Priority:  2,
				},
				ContainerIDForPodUIDAndContName: CollectorRef[ContainerIDForPodUIDAndContNameRetriever]{
					Collector: actualCollector3,
					Priority:  2,
				},
			},
		},
	)

	cID4, err := metaCollector.GetContainerIDForPID(50, 0)
	assert.Equal(t, err, actualCollector3.err)
	assert.Equal(t, "", cID4)

	cIDPodUIDAndContName, err = metaCollector.ContainerIDForPodUIDAndContName("pod3", "foo", false, 0)
	assert.Equal(t, err, actualCollector3.err)
	assert.Equal(t, "", cIDPodUIDAndContName)

	selfCID, err := metaCollector.GetSelfContainerID()
	assert.NoError(t, err)
	assert.Equal(t, "agent1", selfCID)
}
