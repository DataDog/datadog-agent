// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && test

package system

import (
	"context"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
)

func TestSuffixTrieInsertAndGet(t *testing.T) {
	trie := newSuffixTrie()
	cid := "kubelet-kubepods-burstable-pod99dcb84d2a34f7e338778606703258c4.slice/cri-containerd-ec9ea0ad54dd0d96142d5dbe11eb3f1509e12ba9af739620c7b5ad377ce94602"
	cgroupPath := "/host/sys/fs/cgroup/kubelet.slice/kubelet-kubepods.slice/kubelet-kubepods-burstable.slice/kubelet-kubepods-burstable-pod99dcb84d2a34f7e338778606703258c4.slice/cri-containerd-ec9ea0ad54dd0d96142d5dbe11eb3f1509e12ba9af739620c7b5ad377ce94602.scope"
	trie.Insert(cgroupPath, cid)

	assert.Equal(t, cid, trie.Get(cgroupPath), "should return correct container id")
	assert.Equal(t, "", trie.Get("unknown/path"), "should return empty if path is unknown")
}

func TestSuffixTrieDelete_EmptyKey(t *testing.T) {
	trie := newSuffixTrie()
	trie.Delete("")
	trie.Insert("abc", "value")
	assert.Equal(t, trie.Get("abc"), "value", "Deleting empty key should allow to insert keys")
}

func TestSuffixTrieDelete_NonExistentKey(t *testing.T) {
	trie := newSuffixTrie()
	trie.Insert("path/to/container", "container123")
	trie.Delete("nonexistent/key")
	assert.Equal(t, "container123", trie.Get("path/to/container"), "Existing key unaffected by deleting non-existent key")
}

func TestSuffixTrieDelete_RootNode(t *testing.T) {
	trie := newSuffixTrie()
	trie.Insert("", "rootValue")
	trie.Delete("")
	assert.Equal(t, "", trie.Get(""), "Deleting root node should remove the value")
}

func TestSuffixTrieDelete_PartiallyMatchingKey(t *testing.T) {
	trie := newSuffixTrie()
	trie.Insert("path/to/one", "value1")
	trie.Insert("path/to/one/two", "value2")
	trie.Insert("path/to/one/two/three", "value3")
	trie.Delete("path/to/one/two")
	assert.Equal(t, "value1", trie.Get("path/to/one"), "Existing shorter keys should be unaffected")
	assert.Equal(t, "value3", trie.Get("path/to/one/two/three"), "Existing longer keys should be unaffected")
}

func TestHandleSetEvent(t *testing.T) {
	for _, tt := range []struct {
		name               string
		cid                string
		cgroupPath         string
		prefixedCgroupPath string
		cgroupName         string
	}{
		{
			name:               "redis",
			cid:                "redis",
			cgroupPath:         "/default/redis",
			prefixedCgroupPath: "/host/sys/fs/cgroup/default/redis",
			cgroupName:         "redis",
		},
		{
			name:               "kubelet container",
			cid:                "022c4ffba65e5031285fd427553e56c3fd6cc85a3a49f3fa2825d0a258d8a5d6",
			cgroupPath:         "kubelet-kubepods-pod1715d361_61cf_4060_8673_38ab3ca88e66.slice/cri-containerd/022c4ffba65e5031285fd427553e56c3fd6cc85a3a49f3fa2825d0a258d8a5d6",
			prefixedCgroupPath: "/host/sys/fs/cgroup/kubelet.slice/kubelet-kubepods.slice/kubelet-kubepods-burstable.slice/kubelet-kubepods-burstable-pod99dcb84d2a34f7e338778606703258c4.slice/cri-containerd-022c4ffba65e5031285fd427553e56c3fd6cc85a3a49f3fa2825d0a258d8a5d6.scope",
			cgroupName:         "cri-containerd-022c4ffba65e5031285fd427553e56c3fd6cc85a3a49f3fa2825d0a258d8a5d6.scope",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cf := newContainerFilter(nil)
			cont := &workloadmeta.Container{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindContainer,
					ID:   tt.cid,
				},
				CgroupPath: tt.cgroupPath,
			}
			event := workloadmeta.Event{
				Type:   workloadmeta.EventTypeSet,
				Entity: cont,
			}
			cf.handleEvent(event)
			id, err := cf.ContainerFilter(tt.prefixedCgroupPath, tt.cgroupName)
			assert.NoError(t, err)
			assert.Equal(t, tt.cid, id)
		})
	}
}

func TestHandleUnsetEvent(t *testing.T) {
	cf := newContainerFilter(nil)
	cont := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "redis",
		},
		CgroupPath: "/default/redis",
	}
	event := workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: cont,
	}
	cf.handleEvent(event)
	event.Type = workloadmeta.EventTypeUnset
	cf.handleEvent(event)

	id, err := cf.ContainerFilter("/host/sys/fs/cgroup/default/redis", "redis")
	assert.NoError(t, err)
	assert.Equal(t, "", id)
}

func TestListenWorkloadmeta(t *testing.T) {
	wlm := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		logimpl.MockModule(),
		config.MockModule(),
		fx.Supply(context.Background()),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModuleV2(),
	))
	cf := newContainerFilter(wlm)
	go cf.start()
	cont := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "redis",
		},
		CgroupPath: "/default/redis",
	}

	wlm.Set(cont)

	assert.Eventuallyf(t, func() bool {
		cid, _ := cf.ContainerFilter("/host/sys/fs/cgroup/default/redis", "redis")
		return cid == "redis"
	}, 5*time.Second, 200*time.Millisecond, "expected cid to be added to the container filter")
}
