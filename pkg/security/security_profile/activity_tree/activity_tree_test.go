// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func TestInsertFileEvent(t *testing.T) {
	pan := ProcessNode{
		Files: make(map[string]*FileNode),
	}
	pan.Process.FileEvent.PathnameStr = "/test/pan"
	pan.Process.Argv0 = "pan"
	pan.NodeBase = NewNodeBase()
	stats := NewActivityTreeNodeStats()

	pathToInserts := []string{
		"/tmp/foo",
		"/tmp/bar",
		"/test/a/b/c/d/e/",
		"/hello",
		"/tmp/bar/test",
	}
	expectedDebugOuput := strings.TrimSpace(`
- process: /test/pan (argv0: pan) (is_exec_exec:false)
  files:
    - hello
    - test
        - a
            - b
                - c
                    - d
                        - e
    - tmp
        - bar
            - test
        - foo
`)

	for _, path := range pathToInserts {
		event := &model.Event{
			BaseEvent: model.BaseEvent{
				FieldHandlers: &model.FakeFieldHandlers{},
			},
			Open: model.OpenEvent{
				File: model.FileEvent{
					IsPathnameStrResolved: true,
					PathnameStr:           path,
				},
			},
		}
		pan.InsertFileEvent(&event.Open.File, event, "tag", Unknown, stats, false, nil, nil)
	}

	var builder strings.Builder
	pan.debug(&builder, "")
	debugOutput := strings.TrimSpace(builder.String())

	assert.Equal(t, expectedDebugOuput, debugOutput)
}

func setParentRelationship(parent ProcessNodeParent, node *ProcessNode) {
	node.Parent = parent
	for _, child := range node.Children {
		setParentRelationship(node, child)
	}
}

func assertTreeEqual(t *testing.T, wanted *ActivityTree, tree *ActivityTree) {
	var builder strings.Builder
	tree.Debug(&builder)
	inputResult := strings.TrimSpace(builder.String())

	builder.Reset()
	wanted.Debug(&builder)
	wantedResult := strings.TrimSpace(builder.String())

	assert.Equalf(t, wantedResult, inputResult, "the generated tree didn't match the expected output")
}

// activityTreeInsertTestValidator is a mock validator to test the activity tree insert feature
type activityTreeInsertTestValidator struct{}

func (a activityTreeInsertTestValidator) MatchesSelector(entry *model.ProcessCacheEntry) bool {
	return entry.ContainerContext.ContainerID == "123"
}

func (a activityTreeInsertTestValidator) IsEventTypeValid(_ model.EventType) bool {
	return true
}

func (a activityTreeInsertTestValidator) NewProcessNodeCallback(_ *ProcessNode) {}

// newExecTestEventWithAncestors returns a new exec test event with a process cache entry populated with the input list.
// A final `systemd` node is appended.
func newExecTestEventWithAncestors(lineage []model.Process) *model.Event {
	// build the list of ancestors
	ancestor := new(model.ProcessCacheEntry)
	lineageDup := make([]model.Process, len(lineage))
	copy(lineageDup, lineage)

	// reverse lineageDup
	for i, j := 0, len(lineageDup)-1; i < j; i, j = i+1, j-1 {
		lineageDup[i], lineageDup[j] = lineageDup[j], lineageDup[i]
	}

	cursor := ancestor
	maxPid := uint32(len(lineageDup)) + 1

	nextPid := func(current uint32, IsExecExec bool) uint32 {
		if IsExecExec {
			return current
		}
		return current - 1
	}

	currentPid := maxPid - 1
	for _, p := range lineageDup[1:] {
		cursor.Process = p
		cursor.Process.Pid = currentPid
		currentPid = nextPid(currentPid, cursor.Process.IsExecExec)
		cursor.Ancestor = new(model.ProcessCacheEntry)
		cursor.Parent = &cursor.Ancestor.Process
		cursor = cursor.Ancestor
	}

	// append systemd
	cursor.Process = model.Process{
		PIDContext: model.PIDContext{
			Pid: 1,
		},
		FileEvent: model.FileEvent{
			PathnameStr: "/bin/systemd",
			FileFields: model.FileFields{
				PathKey: model.PathKey{
					Inode: math.MaxUint64,
				},
			},
		},
	}

	lineageDup[0].Pid = nextPid(maxPid, !lineageDup[0].IsExecExec)

	evt := &model.Event{
		BaseEvent: model.BaseEvent{
			Type:           uint32(model.ExecEventType),
			FieldHandlers:  &model.FakeFieldHandlers{},
			ProcessContext: &model.ProcessContext{},
			ProcessCacheEntry: &model.ProcessCacheEntry{
				ProcessContext: model.ProcessContext{
					Process:  lineageDup[0],
					Ancestor: ancestor,
					Parent:   &ancestor.Process,
				},
			},
		},
		Exec: model.ExecEvent{
			Process: &model.Process{},
		},
	}
	return evt
}

func TestActivityTree_Patterns(t *testing.T) {
	t.Run("pattern/learning", func(t *testing.T) {
		tree := &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
		}

		// prepare parent links in the input tree
		for _, rootNode := range tree.ProcessNodes {
			setParentRelationship(tree, rootNode)
		}

		event := newExecTestEventWithAncestors([]model.Process{
			{
				ContainerContext: model.ContainerContext{ContainerID: "123"},
				FileEvent: model.FileEvent{
					PathnameStr: "/tmp/123456789/script.sh",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
		})

		wanted := &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/tmp/123456789/script.sh",
						},
					},
				},
			},
		}

		_, newEntry, err := tree.CreateProcessNode(event.ProcessCacheEntry, "tag", Runtime, false, nil)
		assert.NoError(t, err)
		assert.True(t, newEntry)
		assertTreeEqual(t, wanted, tree)

		// add an event that generates a pattern
		event = newExecTestEventWithAncestors([]model.Process{
			{
				ContainerContext: model.ContainerContext{ContainerID: "123"},
				FileEvent: model.FileEvent{
					PathnameStr: "/tmp/987654321/script.sh",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
		})

		wanted = &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/tmp/123456789/script.sh",
						},
					},
				},
			},
		}

		_, newEntry, err = tree.CreateProcessNode(event.ProcessCacheEntry, "tag", Runtime, false, nil)
		assert.NoError(t, err)
		assert.False(t, newEntry)
		assertTreeEqual(t, wanted, tree)
	})

	t.Run("pattern/anamoly", func(t *testing.T) {
		tree := &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
		}

		// prepare parent links in the input tree
		for _, rootNode := range tree.ProcessNodes {
			setParentRelationship(tree, rootNode)
		}

		event := newExecTestEventWithAncestors([]model.Process{
			{
				ContainerContext: model.ContainerContext{ContainerID: "123"},
				FileEvent: model.FileEvent{
					PathnameStr: "/tmp/123456789/script.sh",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
		})

		wanted := &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/tmp/123456789/script.sh",
						},
					},
				},
			},
		}

		_, newEntry, err := tree.CreateProcessNode(event.ProcessCacheEntry, "tag", Runtime, false, nil)
		assert.NoError(t, err)
		assert.True(t, newEntry)
		assertTreeEqual(t, wanted, tree)

		// add an event that generates a pattern
		event = newExecTestEventWithAncestors([]model.Process{
			{
				ContainerContext: model.ContainerContext{ContainerID: "123"},
				FileEvent: model.FileEvent{
					PathnameStr: "/var/123456789/script.sh",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
		})

		wanted = &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/tmp/123456789/script.sh",
						},
					},
				},
			},
		}

		_, newEntry, err = tree.CreateProcessNode(event.ProcessCacheEntry, "tag", Runtime, true, nil)
		assert.NoError(t, err)
		assert.True(t, newEntry)
		assertTreeEqual(t, wanted, tree)
	})
}

func TestEvictUnusedNodes_ProcessCacheProtection(t *testing.T) {
	t.Run("expired_node_gets_evicted_when_not_in_process_cache", func(t *testing.T) {
		// Create an activity tree with a process node that has an old timestamp
		tree := &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
		}

		// Create a process node with an old "last seen" timestamp
		oldTime := time.Now().Add(-2 * time.Hour)
		processNode := &ProcessNode{
			NodeBase: NewNodeBase(),
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/usr/bin/expired",
				},
			},
		}
		processNode.AppendImageTag("test-tag", oldTime)
		tree.ProcessNodes = []*ProcessNode{processNode}

		// Set eviction time to 1 hour ago (node should be evicted)
		evictionTime := time.Now().Add(-1 * time.Hour)

		// Empty process cache (node is not active)
		filepathsInProcessCache := map[ImageProcessKey]bool{}

		// Perform eviction
		evicted := tree.EvictUnusedNodes(evictionTime, filepathsInProcessCache, "test-image", "test-tag")

		// The node should be evicted since it's not in the process cache
		assert.Equal(t, 1, evicted, "Expected 1 node to be evicted")
		assert.Empty(t, tree.ProcessNodes, "Expected process node to be removed from tree")
	})

	t.Run("expired_node_gets_protected_when_in_process_cache", func(t *testing.T) {
		// Create an activity tree with a process node that has an old timestamp
		tree := &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
		}

		// Create a process node with an old "last seen" timestamp
		oldTime := time.Now().Add(-2 * time.Hour)
		processNode := &ProcessNode{
			NodeBase: NewNodeBase(),
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/usr/bin/protected",
				},
			},
		}
		processNode.AppendImageTag("test-tag", oldTime)
		tree.ProcessNodes = []*ProcessNode{processNode}

		// Set eviction time to 1 hour ago (node would normally be evicted)
		evictionTime := time.Now().Add(-1 * time.Hour)

		// Process cache contains this filepath (node is active)
		filepathsInProcessCache := map[ImageProcessKey]bool{
			{ImageName: "test-image", ImageTag: "test-tag", Filepath: "/usr/bin/protected"}: true,
		}

		// Perform eviction
		evicted := tree.EvictUnusedNodes(evictionTime, filepathsInProcessCache, "test-image", "test-tag")

		// The node should NOT be evicted since it's in the process cache
		assert.Equal(t, 0, evicted, "Expected 0 nodes to be evicted")
		assert.Len(t, tree.ProcessNodes, 1, "Expected process node to remain in tree")

		// Verify that the LastSeen timestamp was updated to protect the node
		imageTagTimes := processNode.Seen["test-tag"]
		assert.NotNil(t, imageTagTimes, "Expected image tag to still exist")
		assert.True(t, imageTagTimes.LastSeen.After(evictionTime), "Expected LastSeen to be updated to current time")
	})

	t.Run("mixed_scenario_some_protected_some_evicted", func(t *testing.T) {
		// Create an activity tree with multiple process nodes
		tree := &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
		}

		// Create process nodes with old timestamps
		oldTime := time.Now().Add(-2 * time.Hour)

		protectedNode := &ProcessNode{
			NodeBase: NewNodeBase(),
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/usr/bin/protected",
				},
			},
		}
		protectedNode.AppendImageTag("test-tag", oldTime)

		expiredNode := &ProcessNode{
			NodeBase: NewNodeBase(),
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/usr/bin/expired",
				},
			},
		}
		expiredNode.AppendImageTag("test-tag", oldTime)

		tree.ProcessNodes = []*ProcessNode{protectedNode, expiredNode}

		// Set eviction time to 1 hour ago
		evictionTime := time.Now().Add(-1 * time.Hour)

		// Process cache only contains the protected filepath
		filepathsInProcessCache := map[ImageProcessKey]bool{
			{ImageName: "test-image", ImageTag: "test-tag", Filepath: "/usr/bin/protected"}: true,
		}

		// Perform eviction
		evicted := tree.EvictUnusedNodes(evictionTime, filepathsInProcessCache, "test-image", "test-tag")

		// Only the expired node should be evicted
		assert.Equal(t, 1, evicted, "Expected 1 node to be evicted")
		assert.Len(t, tree.ProcessNodes, 1, "Expected 1 process node to remain in tree")
		assert.Equal(t, "/usr/bin/protected", tree.ProcessNodes[0].Process.FileEvent.PathnameStr, "Expected protected node to remain")

		// Verify that the protected node's timestamp was updated
		imageTagTimes := tree.ProcessNodes[0].Seen["test-tag"]
		assert.NotNil(t, imageTagTimes, "Expected image tag to still exist")
		assert.True(t, imageTagTimes.LastSeen.After(evictionTime), "Expected LastSeen to be updated to current time")
	})

	t.Run("node_with_multiple_image_tags_partial_protection", func(t *testing.T) {
		// Test scenario where a node has multiple image tags, some expired, some not
		tree := &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
		}

		// Create a process node with multiple image tags at different times
		veryOldTime := time.Now().Add(-3 * time.Hour)
		oldTime := time.Now().Add(-2 * time.Hour)
		recentTime := time.Now().Add(-30 * time.Minute)

		processNode := &ProcessNode{
			NodeBase: NewNodeBase(),
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/usr/bin/multi-tag",
				},
			},
		}
		processNode.AppendImageTag("very-old-tag", veryOldTime)
		processNode.AppendImageTag("old-tag", oldTime)
		processNode.AppendImageTag("recent-tag", recentTime)
		processNode.AppendImageTag("test-tag", oldTime) // Add the profile tag that can be refreshed
		tree.ProcessNodes = []*ProcessNode{processNode}

		// Set eviction time to 1 hour ago (very-old-tag and old-tag should be evicted)
		evictionTime := time.Now().Add(-1 * time.Hour)

		// Process cache contains this filepath (node is active)
		filepathsInProcessCache := map[ImageProcessKey]bool{
			{ImageName: "test-image", ImageTag: "test-tag", Filepath: "/usr/bin/multi-tag"}: true,
		}

		// Perform eviction
		evicted := tree.EvictUnusedNodes(evictionTime, filepathsInProcessCache, "test-image", "test-tag")

		// The node should NOT be evicted, but expired tags should be refreshed
		assert.Equal(t, 0, evicted, "Expected 0 nodes to be evicted")
		assert.Len(t, tree.ProcessNodes, 1, "Expected process node to remain in tree")

		// Verify that only the profile's image tag was refreshed
		node := tree.ProcessNodes[0]
		veryOldTagTimes := node.Seen["very-old-tag"]
		oldTagTimes := node.Seen["old-tag"]
		recentTagTimes := node.Seen["recent-tag"]
		testTagTimes := node.Seen["test-tag"]

		// The very-old-tag and old-tag should have been evicted since they weren't refreshed
		assert.Nil(t, veryOldTagTimes, "Expected very-old-tag to be evicted")
		assert.Nil(t, oldTagTimes, "Expected old-tag to be evicted")
		assert.NotNil(t, recentTagTimes, "Expected recent-tag to still exist")
		assert.NotNil(t, testTagTimes, "Expected test-tag to still exist")

		// The test-tag should have been refreshed to current time (it's the profile tag)
		assert.True(t, testTagTimes.LastSeen.After(evictionTime), "Expected test-tag LastSeen to be updated")
		// Recent tag should remain unchanged since it wasn't expired
		assert.True(t, recentTagTimes.LastSeen.Equal(recentTime), "Expected recent-tag LastSeen to remain unchanged")
	})

	t.Run("empty_process_cache_allows_normal_eviction", func(t *testing.T) {
		// Test that when process cache is empty, normal eviction behavior occurs
		tree := &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
		}

		// Create multiple process nodes with old timestamps
		oldTime := time.Now().Add(-2 * time.Hour)

		node1 := &ProcessNode{
			NodeBase: NewNodeBase(),
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/usr/bin/node1",
				},
			},
		}
		node1.AppendImageTag("test-tag", oldTime)

		node2 := &ProcessNode{
			NodeBase: NewNodeBase(),
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/usr/bin/node2",
				},
			},
		}
		node2.AppendImageTag("test-tag", oldTime)

		tree.ProcessNodes = []*ProcessNode{node1, node2}

		// Set eviction time to 1 hour ago
		evictionTime := time.Now().Add(-1 * time.Hour)

		// Empty process cache
		filepathsInProcessCache := map[ImageProcessKey]bool{}

		// Perform eviction
		evicted := tree.EvictUnusedNodes(evictionTime, filepathsInProcessCache, "test-image", "test-tag")

		// Both nodes should be evicted
		assert.Equal(t, 2, evicted, "Expected 2 nodes to be evicted")
		assert.Empty(t, tree.ProcessNodes, "Expected all process nodes to be removed from tree")
	})
}
