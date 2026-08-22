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
	"github.com/stretchr/testify/require"

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
		_, _ = pan.InsertFileEvent(&event.Open.File, event, uint64(666), Unknown, stats, false, nil, nil)
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

		testTagID := tree.GetOrInsertImageTag("test-tag")

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
		processNode.AppendImageTagID(testTagID, oldTime)
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

		testTagID := tree.GetOrInsertImageTag("test-tag")

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

		processNode.AppendImageTagID(testTagID, oldTime)
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
		imageTagTimes, exists := processNode.GetSeenTimes(testTagID)
		assert.True(t, exists, "Expected image tag to still exist")
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

		testTagID := tree.GetOrInsertImageTag("test-tag")

		protectedNode := &ProcessNode{
			NodeBase: NewNodeBase(),
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/usr/bin/protected",
				},
			},
		}

		protectedNode.AppendImageTagID(testTagID, oldTime)

		expiredNode := &ProcessNode{
			NodeBase: NewNodeBase(),
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/usr/bin/expired",
				},
			},
		}

		expiredNode.AppendImageTagID(testTagID, oldTime)

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
		imageTagTimes, exists := tree.ProcessNodes[0].GetSeenTimes(testTagID)
		assert.True(t, exists, "Expected image tag to still exist")
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

		veryOldTagID := tree.GetOrInsertImageTag("very-old-tag")
		oldTagID := tree.GetOrInsertImageTag("old-tag")
		recentTagID := tree.GetOrInsertImageTag("recent-tag")
		testTagID := tree.GetOrInsertImageTag("test-tag")

		processNode := &ProcessNode{
			NodeBase: NewNodeBase(),
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/usr/bin/multi-tag",
				},
			},
		}

		processNode.AppendImageTagID(veryOldTagID, veryOldTime)
		processNode.AppendImageTagID(oldTagID, oldTime)
		processNode.AppendImageTagID(recentTagID, recentTime)
		processNode.AppendImageTagID(testTagID, oldTime) // Add the profile tag that can be refreshed
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
		veryOldTagTimes, _ := node.GetSeenTimes(veryOldTagID)
		oldTagTimes, _ := node.GetSeenTimes(oldTagID)
		recentTagTimes, _ := node.GetSeenTimes(recentTagID)
		testTagTimes, _ := node.GetSeenTimes(testTagID)

		// The very-old-tag and old-tag should have been evicted since they weren't refreshed
		assert.Zero(t, veryOldTagTimes, "Expected very-old-tag to be evicted")
		assert.Zero(t, oldTagTimes, "Expected old-tag to be evicted")
		assert.NotZero(t, recentTagTimes, "Expected recent-tag to still exist")
		assert.NotZero(t, testTagTimes, "Expected test-tag to still exist")

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
		testTagID := uint64(666)
		node1.AppendImageTagID(testTagID, oldTime)

		node2 := &ProcessNode{
			NodeBase: NewNodeBase(),
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/usr/bin/node2",
				},
			},
		}
		node2.AppendImageTagID(testTagID, oldTime)

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

// Syscall and capability nodes must survive a time based eviction pass that does prune a sibling.
func TestEvictUnusedNodes_SyscallAndCapabilityExemption(t *testing.T) {
	tree := &ActivityTree{
		validator:    activityTreeInsertTestValidator{},
		Stats:        NewActivityTreeNodeStats(),
		SyscallsMask: make(map[int]int),
	}

	testTagID := tree.GetOrInsertImageTag("test-tag")
	oldTime := time.Now().Add(-2 * time.Hour)

	processNode := &ProcessNode{
		NodeBase: NewNodeBase(),
		Process: model.Process{
			FileEvent: model.FileEvent{
				PathnameStr: "/usr/bin/exempt",
			},
		},
		DNSNames: make(map[string]*DNSNode),
	}
	processNode.AppendImageTagID(testTagID, oldTime)

	// all three children are equally stale
	processNode.Syscalls = []*SyscallNode{
		NewSyscallNode(42, oldTime, testTagID, Runtime),
	}
	processNode.Capabilities = []*CapabilityNode{
		NewCapabilityNode(7, true, oldTime, testTagID, Runtime),
	}
	dnsNode := &DNSNode{
		NodeBase:       NewNodeBase(),
		GenerationType: Runtime,
		Requests:       []model.DNSEvent{{Question: model.DNSQuestion{Name: "example.com"}}},
	}
	dnsNode.AppendImageTagID(testTagID, oldTime)
	processNode.DNSNames["example.com"] = dnsNode

	tree.ProcessNodes = []*ProcessNode{processNode}

	// keep the parent process node alive so we only observe child eviction
	filepathsInProcessCache := map[ImageProcessKey]bool{
		{ImageName: "test-image", ImageTag: "test-tag", Filepath: "/usr/bin/exempt"}: true,
	}

	tree.EvictUnusedNodes(time.Now().Add(-1*time.Hour), filepathsInProcessCache, "test-image", "test-tag")

	require.Len(t, tree.ProcessNodes, 1, "the parent process node should be protected by the process cache")
	node := tree.ProcessNodes[0]

	assert.Len(t, node.Syscalls, 1, "stale syscall nodes must not be evicted")
	assert.Len(t, node.Capabilities, 1, "stale capability nodes must not be evicted")
	assert.Empty(t, node.DNSNames, "a stale DNS node should still be evicted, proving the pass ran")
}

// The kernel delivers a syscall mask that only grows and is never reset between sends, so re-delivering
// an unchanged mask must report no new syscalls.
func TestInsertSyscalls_AccumulatingMask(t *testing.T) {
	newSyscallsEvent := func(syscalls ...int) *model.Event {
		evt := &model.Event{
			BaseEvent: model.BaseEvent{FieldHandlers: &model.FakeFieldHandlers{}},
		}
		for _, s := range syscalls {
			evt.Syscalls.Syscalls = append(evt.Syscalls.Syscalls, model.Syscall(s))
		}
		return evt
	}

	pn := &ProcessNode{NodeBase: NewNodeBase()}
	stats := NewActivityTreeNodeStats()
	syscallMask := make(map[int]int)
	const tagID = uint64(1)

	assert.True(t, pn.InsertSyscalls(newSyscallsEvent(1, 2), tagID, syscallMask, stats, false),
		"the first delivery introduces new syscalls")
	assert.Len(t, pn.Syscalls, 2)

	assert.False(t, pn.InsertSyscalls(newSyscallsEvent(1, 2), tagID, syscallMask, stats, false),
		"re-delivering the same mask must not report new syscalls")
	assert.Len(t, pn.Syscalls, 2, "re-delivery must not duplicate nodes")

	assert.True(t, pn.InsertSyscalls(newSyscallsEvent(1, 2, 3), tagID, syscallMask, stats, false),
		"a grown mask reports the newly discovered syscall")
	assert.Len(t, pn.Syscalls, 3, "only the genuinely new syscall is added")
	assert.Equal(t, map[int]int{1: 1, 2: 2, 3: 3}, syscallMask)
}

func TestSyscallsByImageTagID(t *testing.T) {
	tree := NewActivityTree(activityTreeInsertTestValidator{}, nil, "security_profile")

	v1 := tree.GetOrInsertImageTag("v1")
	v2 := tree.GetOrInsertImageTag("v2")
	now := time.Now()

	// A syscall shared by both processes of v1, one exclusive to each tag, and a node carrying
	// both tags at once.
	parent := &ProcessNode{NodeBase: NewNodeBase()}
	parent.Syscalls = []*SyscallNode{
		NewSyscallNode(1, now, v1, Runtime),
		NewSyscallNode(60, now, v2, Runtime),
	}
	child := &ProcessNode{NodeBase: NewNodeBase()}
	child.Syscalls = []*SyscallNode{
		NewSyscallNode(1, now, v1, Runtime),
		NewSyscallNode(2, now, v1, Runtime),
	}
	shared := NewSyscallNode(257, now, v1, Runtime)
	shared.AppendImageTagID(v2, now)
	child.Syscalls = append(child.Syscalls, shared)

	parent.Children = []*ProcessNode{child}
	tree.ProcessNodes = []*ProcessNode{parent}

	rollup := tree.SyscallsByImageTagID()

	assert.Equal(t, []uint32{1, 2, 257}, rollup[v1], "v1 unions both processes and dedups syscall 1")
	assert.Equal(t, []uint32{60, 257}, rollup[v2], "v2 only sees its own syscalls plus the shared node")
}

func TestCapabilitiesByImageTagID(t *testing.T) {
	tree := NewActivityTree(activityTreeInsertTestValidator{}, nil, "security_profile")

	v1 := tree.GetOrInsertImageTag("v1")
	v2 := tree.GetOrInsertImageTag("v2")
	now := time.Now()

	// Capability 7 was checked for and held, 12 was checked for but not held, so it is attempted
	// only. Capability 21 lands on both tags.
	parent := &ProcessNode{NodeBase: NewNodeBase()}
	parent.Capabilities = []*CapabilityNode{
		NewCapabilityNode(7, true, now, v1, Runtime),
		NewCapabilityNode(12, false, now, v1, Runtime),
	}
	child := &ProcessNode{NodeBase: NewNodeBase()}
	child.Capabilities = []*CapabilityNode{
		NewCapabilityNode(7, true, now, v1, Runtime),
		NewCapabilityNode(30, true, now, v2, Runtime),
	}
	shared := NewCapabilityNode(21, true, now, v1, Runtime)
	shared.AppendImageTagID(v2, now)
	child.Capabilities = append(child.Capabilities, shared)

	parent.Children = []*ProcessNode{child}
	tree.ProcessNodes = []*ProcessNode{parent}

	rollup := tree.CapabilitiesByImageTagID()

	assert.Equal(t, []uint64{7, 12, 21}, rollup[v1].Attempted, "v1 unions both processes and dedups capability 7")
	assert.Equal(t, []uint64{7, 21}, rollup[v1].Used, "capability 12 was attempted but never held")
	assert.Equal(t, []uint64{21, 30}, rollup[v2].Attempted)
	assert.Equal(t, []uint64{21, 30}, rollup[v2].Used)
}

// The same capability yields two nodes when a process is checked for it both with and without
// holding it, and the rollup has to report it as attempted and used exactly once.
func TestCapabilitiesByImageTagID_AttemptedAndUsedSameCapability(t *testing.T) {
	tree := NewActivityTree(activityTreeInsertTestValidator{}, nil, "security_profile")

	v1 := tree.GetOrInsertImageTag("v1")
	now := time.Now()

	pn := &ProcessNode{NodeBase: NewNodeBase()}
	pn.Capabilities = []*CapabilityNode{
		NewCapabilityNode(7, false, now, v1, Runtime),
		NewCapabilityNode(7, true, now, v1, Runtime),
	}
	tree.ProcessNodes = []*ProcessNode{pn}

	rollup := tree.CapabilitiesByImageTagID()

	assert.Equal(t, []uint64{7}, rollup[v1].Attempted)
	assert.Equal(t, []uint64{7}, rollup[v1].Used)
}

// A node that only belongs to an evicted image tag must drop out of that tag's rollup.
func TestSyscallsByImageTagID_AfterImageTagEviction(t *testing.T) {
	tree := NewActivityTree(activityTreeInsertTestValidator{}, nil, "security_profile")

	v1 := tree.GetOrInsertImageTag("v1")
	now := time.Now()

	pn := &ProcessNode{NodeBase: NewNodeBase()}
	pn.AppendImageTagID(v1, now)
	pn.Syscalls = []*SyscallNode{NewSyscallNode(42, now, v1, Runtime)}
	tree.ProcessNodes = []*ProcessNode{pn}

	require.Equal(t, []uint32{42}, tree.SyscallsByImageTagID()[v1])

	tree.EvictImageTag("v1")

	assert.Empty(t, tree.SyscallsByImageTagID()[v1], "an evicted image tag keeps no syscalls")
}
