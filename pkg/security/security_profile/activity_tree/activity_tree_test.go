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

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func TestInsertFileEvent(t *testing.T) {
	pan := ProcessNode{
		Files: make(map[string]*FileNode),
	}
	pan.Process.FileEvent.PathnameStr = "/test/pan"
	pan.Process.Argv0 = "pan"
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
	return entry.ContainerID == "123"
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
			Type:             uint32(model.ExecEventType),
			FieldHandlers:    &model.FakeFieldHandlers{},
			ContainerContext: &model.ContainerContext{},
			ProcessContext:   &model.ProcessContext{},
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
				ContainerID: "123",
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
				ContainerID: "123",
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
				ContainerID: "123",
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
				ContainerID: "123",
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
