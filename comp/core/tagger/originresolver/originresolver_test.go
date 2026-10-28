// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package originresolver

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	metricsmock "github.com/DataDog/datadog-agent/pkg/util/containers/metrics/mock"
)

// countingMetaCollector counts the lookups it serves.
type countingMetaCollector struct {
	metricsmock.MetaCollector
	inodeCalls        int
	externalDataCalls int
}

func (c *countingMetaCollector) GetContainerIDForInode(inode uint64, cacheValidity time.Duration) (string, error) {
	c.inodeCalls++
	return c.MetaCollector.GetContainerIDForInode(inode, cacheValidity)
}

func (c *countingMetaCollector) ContainerIDForPodUIDAndContName(podUID, contName string, initCont bool, cacheValidity time.Duration) (string, error) {
	c.externalDataCalls++
	return c.MetaCollector.ContainerIDForPodUIDAndContName(podUID, contName, initCont, cacheValidity)
}

func newCountingMetaCollector() *countingMetaCollector {
	return &countingMetaCollector{MetaCollector: metricsmock.MetaCollector{
		CIDFromInode:          map[uint64]string{1: "inode-cid"},
		CIDFromPodUIDContName: map[string]string{"pod/app": "external-cid", "i-pod/init": "init-cid"},
	}}
}

func TestContainerIDFromSocket(t *testing.T) {
	assert.Equal(t, "cid", ContainerIDFromSocket(types.NewEntityID(types.ContainerID, "cid").String()))
	assert.Equal(t, "", ContainerIDFromSocket(""))
	assert.Equal(t, "", ContainerIDFromSocket("container_id://"))
}

func TestContainerIDForInodeAndExternalData(t *testing.T) {
	mc := newCountingMetaCollector()

	cid, err := ContainerIDForInode(1, mc)
	assert.NoError(t, err)
	assert.Equal(t, "inode-cid", cid)

	cid, err = ContainerIDForExternalData(origindetection.ExternalData{PodUID: "pod", ContainerName: "init", Init: true}, mc)
	assert.NoError(t, err)
	assert.Equal(t, "init-cid", cid)

	// Unset or incomplete data is not looked up
	cid, _ = ContainerIDForInode(0, mc)
	assert.Empty(t, cid)
	cid, _ = ContainerIDForExternalData(origindetection.ExternalData{PodUID: "pod"}, mc)
	assert.Empty(t, cid)
	assert.Equal(t, 1, mc.inodeCalls)
	assert.Equal(t, 1, mc.externalDataCalls)

	// No meta collector
	cid, err = ContainerIDForInode(1, nil)
	assert.NoError(t, err)
	assert.Empty(t, cid)
}

func TestReuseResolvedOrigin(t *testing.T) {
	mc := newCountingMetaCollector()
	origin := taggertypes.OriginInfo{
		LocalData:    origindetection.LocalData{Inode: 1},
		ExternalData: origindetection.ExternalData{PodUID: "pod", ContainerName: "app"},
		// Failed resolutions are reused too
		Resolved: &taggertypes.ResolvedOrigin{InodeDone: true, ExternalDataContainerID: "already-resolved", ExternalDataDone: true},
	}

	cid, _ := InodeContainerID(origin, mc)
	assert.Empty(t, cid)
	cid, _ = ExternalDataContainerID(origin, mc)
	assert.Equal(t, "already-resolved", cid)
	assert.Zero(t, mc.inodeCalls)
	assert.Zero(t, mc.externalDataCalls)

	// Not resolved yet: looked up
	origin.Resolved = nil
	cid, _ = InodeContainerID(origin, mc)
	assert.Equal(t, "inode-cid", cid)
	cid, _ = ExternalDataContainerID(origin, mc)
	assert.Equal(t, "external-cid", cid)
}

func TestResolve(t *testing.T) {
	for _, tt := range []struct {
		name                      string
		origin                    taggertypes.OriginInfo
		expectedContainerID       string
		expectedResolved          *taggertypes.ResolvedOrigin
		expectedInodeCalls        int
		expectedExternalDataCalls int
	}{
		{
			name:                "no origin",
			origin:              taggertypes.OriginInfo{},
			expectedContainerID: "",
		},
		{
			name: "UDS origin first, without lookups",
			origin: taggertypes.OriginInfo{
				ContainerIDFromSocket: types.NewEntityID(types.ContainerID, "socket-cid").String(),
				LocalData:             origindetection.LocalData{ContainerID: "local-cid", Inode: 1},
			},
			expectedContainerID: "socket-cid",
		},
		{
			name:                "client container ID before inode",
			origin:              taggertypes.OriginInfo{LocalData: origindetection.LocalData{ContainerID: "local-cid", Inode: 1}},
			expectedContainerID: "local-cid",
		},
		{
			name: "inode before external data",
			origin: taggertypes.OriginInfo{
				LocalData:    origindetection.LocalData{Inode: 1},
				ExternalData: origindetection.ExternalData{PodUID: "pod", ContainerName: "app"},
			},
			expectedContainerID: "inode-cid",
			expectedResolved:    &taggertypes.ResolvedOrigin{InodeContainerID: "inode-cid", InodeDone: true},
			expectedInodeCalls:  1,
		},
		{
			name: "external data when the inode is unknown",
			origin: taggertypes.OriginInfo{
				LocalData:    origindetection.LocalData{Inode: 42},
				ExternalData: origindetection.ExternalData{PodUID: "pod", ContainerName: "app"},
			},
			expectedContainerID:       "external-cid",
			expectedResolved:          &taggertypes.ResolvedOrigin{InodeDone: true, ExternalDataContainerID: "external-cid", ExternalDataDone: true},
			expectedInodeCalls:        1,
			expectedExternalDataCalls: 1,
		},
		{
			name:                      "failed lookups are returned",
			origin:                    taggertypes.OriginInfo{ExternalData: origindetection.ExternalData{PodUID: "unknown", ContainerName: "app"}},
			expectedContainerID:       "",
			expectedResolved:          &taggertypes.ResolvedOrigin{ExternalDataDone: true},
			expectedExternalDataCalls: 1,
		},
		{
			name:                "pod-only origin has no container",
			origin:              taggertypes.OriginInfo{LocalData: origindetection.LocalData{PodUID: "pod"}},
			expectedContainerID: "",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mc := newCountingMetaCollector()
			cid, resolved := Resolve(tt.origin, mc)
			assert.Equal(t, tt.expectedContainerID, cid)
			assert.Equal(t, tt.expectedResolved, resolved)
			assert.Equal(t, tt.expectedInodeCalls, mc.inodeCalls)
			assert.Equal(t, tt.expectedExternalDataCalls, mc.externalDataCalls)
		})
	}
}

func TestResolveWithoutMetaCollector(t *testing.T) {
	origin := taggertypes.OriginInfo{LocalData: origindetection.LocalData{Inode: 1}}
	cid, resolved := Resolve(origin, nil)
	assert.Empty(t, cid)
	assert.Nil(t, resolved, "lookups that were not done must not be reported as done")
}
