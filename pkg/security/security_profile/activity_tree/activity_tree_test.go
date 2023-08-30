// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"fmt"
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
	stats := NewActivityTreeNodeStats()

	pathToInserts := []string{
		"/tmp/foo",
		"/tmp/bar",
		"/test/a/b/c/d/e/",
		"/hello",
		"/tmp/bar/test",
	}
	expectedDebugOuput := strings.TrimSpace(`
- process: /test/pan (is_exec_child:false)
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
				FieldHandlers: &model.DefaultFieldHandlers{},
			},
			Open: model.OpenEvent{
				File: model.FileEvent{
					IsPathnameStrResolved: true,
					PathnameStr:           path,
				},
			},
		}
		pan.InsertFileEvent(&event.Open.File, event, Unknown, stats, false, nil, nil)
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

func TestActivityTree_InsertExecEvent(t *testing.T) {
	for _, tt := range activityTreeInsertExecEventTestCases {
		t.Run(tt.name, func(t *testing.T) {
			// prepare parent links in the input tree
			for _, rootNode := range tt.tree.ProcessNodes {
				setParentRelationship(tt.tree, rootNode)
			}

			node, newEntry, err := tt.tree.CreateProcessNode(tt.inputEvent.ProcessCacheEntry, Runtime, false, nil)
			if tt.wantErr != nil {
				if !tt.wantErr(t, err, fmt.Sprintf("unexpected error: %v", err)) {
					return
				}
			} else if err != nil {
				t.Fatalf("an err was returned but none was expected: %v", err)
				return
			}

			assertTreeEqual(t, tt.wantTree, tt.tree)

			assert.Equalf(t, tt.wantNewEntry, newEntry, "invalid newEntry output")
			assert.Equalf(t, tt.wantNode.Process.FileEvent.PathnameStr, node.Process.FileEvent.PathnameStr, "the returned ProcessNode is invalid")
		})
	}
}

// activityTreeInsertTestValidator is a mock validator to test the activity tree insert feature
type activityTreeInsertTestValidator struct{}

func (a activityTreeInsertTestValidator) MatchesSelector(entry *model.ProcessCacheEntry) bool {
	return entry.ContainerID == "123"
}

func (a activityTreeInsertTestValidator) IsEventTypeValid(evtType model.EventType) bool {
	return true
}

func (a activityTreeInsertTestValidator) NewProcessNodeCallback(p *ProcessNode) {}

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
	for _, p := range lineageDup[1:] {
		cursor.Process = p
		cursor.Ancestor = new(model.ProcessCacheEntry)
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

	evt := &model.Event{
		BaseEvent: model.BaseEvent{
			Type:             uint32(model.ExecEventType),
			FieldHandlers:    &model.DefaultFieldHandlers{},
			ContainerContext: &model.ContainerContext{},
			ProcessContext:   &model.ProcessContext{},
			ProcessCacheEntry: &model.ProcessCacheEntry{
				ProcessContext: model.ProcessContext{
					Process:  lineageDup[0],
					Ancestor: ancestor,
				},
			},
		},
		Exec: model.ExecEvent{
			Process: &model.Process{},
		},
	}
	return evt
}

var activityTreeInsertExecEventTestCases = []struct {
	name         string
	tree         *ActivityTree
	inputEvent   *model.Event
	wantNewEntry bool
	wantErr      assert.ErrorAssertionFunc
	wantTree     *ActivityTree
	wantNode     *ProcessNode
}{
	// exec/1
	// ---------------
	//
	//     empty tree          +          systemd                 ==>>              /bin/bash
	//                                       |- /bin/bash                               |
	//                                       |- /bin/ls                              /bin/ls
	{
		name: "exec/1",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: true,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/ls",
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/2
	// ---------------
	//
	//      /bin/bash          +          systemd                 ==>>              /bin/bash
	//                                       |- /bin/bash                               |
	//                                       |- /bin/ls                              /bin/ls
	{
		name: "exec/2",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: true,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/ls",
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/3
	// ---------------
	//
	//      /bin/bash          +          systemd                 ==>>              /bin/bash ------------
	//          |                            |- /bin/bash                               |                |
	//      /bin/webserver                   |- /bin/ls                           /bin/webserver      /bin/ls
	//          |                                                                       |
	//       /bin/ls                                                                 /bin/ls
	{
		name: "exec/3",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/ls",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: true,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/ls",
										},
									},
								},
							},
						},
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/ls",
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/4
	// ---------------
	//
	//      /bin/bash          +          systemd                 ==>>              /bin/bash
	//          |                            |- /bin/bash                               |
	//      /bin/webserver                   |- /bin/ls                            /bin/webserver
	//          | (exec)                                                                | (exec)
	//       /bin/ls                                                                 /bin/ls
	{
		name: "exec/4",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/ls",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: false,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/ls",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/4_bis
	// ---------------
	//
	//      /bin/bash          +          systemd                 ==>>              /bin/bash
	//          |                            |- /bin/bash                               |
	//      /bin/webserver---                |- /bin/ls                            /bin/webserver-----
	//          | (exec)    | (exec)                                                    | (exec)     | (exec)
	//       /bin/id     /bin/ls                                                     /bin/id      /bin/ls
	{
		name: "exec/4_bis",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/id",
										},
									},
								},
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/ls",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: false,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/id",
										},
									},
								},
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/ls",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/5
	// ---------------
	//
	//      /bin/webserver         +          systemd             ==>>           /bin/webserver
	//          | (exec)                       |- /bin/ls                              | (exec)
	//       /bin/ls                                                                 /bin/ls
	{
		name: "exec/5",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/webserver",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								IsExecChild: true,
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/ls",
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: false,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/webserver",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								IsExecChild: true,
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/ls",
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/6
	// ---------------
	//
	//      /bin/bash          +          systemd                 ==>>               /bin/bash
	//          |                            |- /bin/bash                               |
	//      /bin/webserver1                  |- /bin/ls                           /bin/webserver1
	//          | (exec)                                                                | (exec)
	//     /bin/webserver2----------                                              /bin/webserver2---------
	//          | (exec)           |                                                    | (exec)         |
	//     /bin/webserver3      /bin/id                                           /bin/webserver3      /bin/id
	//          | (exec)                                                                | (exec)
	//     /bin/webserver4                                                        /bin/webserver4
	//          | (exec)                                                                | (exec)
	//       /bin/ls---------------                                                  /bin/ls--------------
	//          |                 |                                                     |                |
	//       /bin/wc           /bin/id                                               /bin/wc          /bin/id
	{
		name: "exec/6",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver1",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/webserver2",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/id",
												},
											},
										},
										{
											Process: model.Process{
												IsExecChild: true,
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/webserver3",
												},
											},
											Children: []*ProcessNode{
												{
													Process: model.Process{
														IsExecChild: true,
														FileEvent: model.FileEvent{
															PathnameStr: "/bin/webserver4",
														},
													},
													Children: []*ProcessNode{
														{
															Process: model.Process{
																IsExecChild: true,
																FileEvent: model.FileEvent{
																	PathnameStr: "/bin/ls",
																},
															},
															Children: []*ProcessNode{
																{
																	Process: model.Process{
																		FileEvent: model.FileEvent{
																			PathnameStr: "/bin/id",
																		},
																	},
																},
																{
																	Process: model.Process{
																		FileEvent: model.FileEvent{
																			PathnameStr: "/bin/wc",
																		},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: false,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver1",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/webserver2",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/id",
												},
											},
										},
										{
											Process: model.Process{
												IsExecChild: true,
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/webserver3",
												},
											},
											Children: []*ProcessNode{
												{
													Process: model.Process{
														IsExecChild: true,
														FileEvent: model.FileEvent{
															PathnameStr: "/bin/webserver4",
														},
													},
													Children: []*ProcessNode{
														{
															Process: model.Process{
																IsExecChild: true,
																FileEvent: model.FileEvent{
																	PathnameStr: "/bin/ls",
																},
															},
															Children: []*ProcessNode{
																{
																	Process: model.Process{
																		FileEvent: model.FileEvent{
																			PathnameStr: "/bin/id",
																		},
																	},
																},
																{
																	Process: model.Process{
																		FileEvent: model.FileEvent{
																			PathnameStr: "/bin/wc",
																		},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/7
	// ---------------
	//
	//      /bin/webserver1              +           systemd           ==>        /bin/webserver1
	//          | (exec)                              |- /bin/ls                        | (exec)
	//     /bin/webserver2----------                                              /bin/webserver2---------
	//          | (exec)           |                                                    | (exec)         |
	//     /bin/webserver3      /bin/id                                           /bin/webserver3     /bin/id
	//          | (exec)                                                                | (exec)
	//     /bin/webserver4                                                        /bin/webserver4
	//          | (exec)                                                                | (exec)
	//       /bin/ls---------------                                                  /bin/ls--------------
	//          |                 |                                                     |                |
	//       /bin/wc           /bin/id                                               /bin/wc          /bin/id
	{
		name: "exec/7",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/webserver1",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								IsExecChild: true,
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver2",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/id",
										},
									},
								},
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/webserver3",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												IsExecChild: true,
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/webserver4",
												},
											},
											Children: []*ProcessNode{
												{
													Process: model.Process{
														IsExecChild: true,
														FileEvent: model.FileEvent{
															PathnameStr: "/bin/ls",
														},
													},
													Children: []*ProcessNode{
														{
															Process: model.Process{
																FileEvent: model.FileEvent{
																	PathnameStr: "/bin/id",
																},
															},
														},
														{
															Process: model.Process{
																FileEvent: model.FileEvent{
																	PathnameStr: "/bin/wc",
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: false,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/webserver1",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								IsExecChild: true,
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver2",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/id",
										},
									},
								},
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/webserver3",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												IsExecChild: true,
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/webserver4",
												},
											},
											Children: []*ProcessNode{
												{
													Process: model.Process{
														IsExecChild: true,
														FileEvent: model.FileEvent{
															PathnameStr: "/bin/ls",
														},
													},
													Children: []*ProcessNode{
														{
															Process: model.Process{
																FileEvent: model.FileEvent{
																	PathnameStr: "/bin/id",
																},
															},
														},
														{
															Process: model.Process{
																FileEvent: model.FileEvent{
																	PathnameStr: "/bin/wc",
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/8
	// ---------------
	//
	//      /bin/bash          +          systemd                              ==>>              /bin/bash
	//          |                            |- /bin/bash                                             |
	//      /bin/ls                          |- /bin/webserver -> /bin/ls                       /bin/webserver
	//                                                                                                | (exec)
	//                                                                                             /bin/ls
	{
		name: "exec/8",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/ls",
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/webserver",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
			{
				IsExecChild: true,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 3,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: true,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/ls",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/9
	// ---------------
	//
	//      /bin/webserver      +          systemd                              ==>>              /bin/bash
	//          |                            |- /bin/bash -> /bin/webserver                           | (exec)
	//      /bin/ls                          |- /bin/ls                                         /bin/webserver
	//                                                                                                |
	//                                                                                             /bin/ls
	{
		name: "exec/9",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/webserver",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/ls",
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
			{
				IsExecChild: true,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/webserver",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 3,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: true,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								IsExecChild: true,
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/ls",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/10
	// ---------------
	//
	//      /bin/webserver      +          systemd                                              ==>>              /bin/bash
	//          |                            |- /bin/bash -> /bin/webserver -> /bin/apache                           | (exec)
	//      /bin/ls                          |- /bin/ls                                                         /bin/webserver------------
	//                                                                                                               | (exec)            |
	//                                                                                                          /bin/apache           /bin/ls
	//                                                                                                               |
	//                                                                                                            /bin/ls
	{
		name: "exec/10",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/webserver",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/ls",
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
			{
				IsExecChild: true,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/webserver",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
			{
				IsExecChild: true,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/apache",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 3,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 4,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: true,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								IsExecChild: true,
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/ls",
										},
									},
								},
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/apache",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/ls",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/11
	// ---------------
	//
	//      /bin/apache         +          systemd                                              ==>>              /bin/bash
	//          |                            |- /bin/bash -> /bin/webserver -> /bin/apache                           | (exec)
	//      /bin/ls                          |- /bin/ls                                                         /bin/webserver
	//                                                                                                               | (exec)
	//                                                                                                          /bin/apache
	//                                                                                                               |
	//                                                                                                            /bin/ls
	{
		name: "exec/11",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/apache",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/ls",
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
			{
				IsExecChild: true,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/webserver",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
			{
				IsExecChild: true,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/apache",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 3,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 4,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: true,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								IsExecChild: true,
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/apache",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/ls",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/12
	// ---------------
	//
	//      /bin/apache         +          systemd                                              ==>>              /bin/bash
	//          |                            |- /bin/bash -> /bin/webserver -> /bin/apache                           | (exec)
	//       /bin/ls                         |- /bin/wc -> /bin/id -> /bin/ls                                   /bin/webserver
	//          |                            |- /bin/date                                                            | (exec)
	//       /bin/date                       |- /bin/passwd -> /bin/bpftool -> /bin/du                           /bin/apache
	//          |                                                                                                     |
	//       /bin/du                                                                                               /bin/wc
	//                                                                                                               | (exec)
	//                                                                                                            /bin/id
	//                                                                                                               | (exec)
	//                                                                                                            /bin/ls
	//                                                                                                               |
	//                                                                                                            /bin/date
	//                                                                                                               |
	//                                                                                                           /bin/passwd
	//                                                                                                               | (exec)
	//                                                                                                           /bin/bpftool
	//                                                                                                               | (exec)
	//                                                                                                           /bin/du
	{
		name: "exec/12",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/apache",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/ls",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/date",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/du",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
			{
				IsExecChild: true,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/webserver",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
			{
				IsExecChild: true,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/apache",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 3,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/wc",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 4,
						},
					},
				},
			},
			{
				IsExecChild: true,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/id",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 5,
						},
					},
				},
			},
			{
				IsExecChild: true,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 6,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/date",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 7,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/passwd",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 8,
						},
					},
				},
			},
			{
				IsExecChild: true,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bpftool",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 9,
						},
					},
				},
			},
			{
				IsExecChild: true,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/du",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 10,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/du",
				},
			},
		},
		wantNewEntry: true,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								IsExecChild: true,
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/apache",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/wc",
												},
											},
											Children: []*ProcessNode{
												{
													Process: model.Process{
														IsExecChild: true,
														FileEvent: model.FileEvent{
															PathnameStr: "/bin/id",
														},
													},
													Children: []*ProcessNode{
														{
															Process: model.Process{
																IsExecChild: true,
																FileEvent: model.FileEvent{
																	PathnameStr: "/bin/ls",
																},
															},
															Children: []*ProcessNode{
																{
																	Process: model.Process{
																		FileEvent: model.FileEvent{
																			PathnameStr: "/bin/date",
																		},
																	},
																	Children: []*ProcessNode{
																		{
																			Process: model.Process{
																				FileEvent: model.FileEvent{
																					PathnameStr: "/bin/passwd",
																				},
																			},
																			Children: []*ProcessNode{
																				{
																					Process: model.Process{
																						IsExecChild: true,
																						FileEvent: model.FileEvent{
																							PathnameStr: "/bin/bpftool",
																						},
																					},
																					Children: []*ProcessNode{
																						{
																							Process: model.Process{
																								IsExecChild: true,
																								FileEvent: model.FileEvent{
																									PathnameStr: "/bin/du",
																								},
																							},
																						},
																					},
																				},
																			},
																		},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/12_bis
	// ---------------
	//
	//      /bin/apache         +          systemd                                              ==>>              /bin/bash          /bin/apache
	//          |                            |- /bin/bash                                                            |                    |
	//       /bin/ls                         |- /bin/webserver                                                 /bin/webserver          /bin/ls
	//          |                            |- /bin/apache                                                          |                    |
	//       /bin/date                       |- /bin/wc                                                         /bin/apache           /bin/date
	//          |                            |- /bin/id                                                              |                    |
	//       /bin/du                         |- /bin/ls                                                           /bin/wc              /bin/du
	//                                       |- /bin/date                                                            |
	//                                       |- /bin/passwd                                                       /bin/id
	//                                       |- /bin/bpftool                                                         |
	//                                       |- /bin/du                                                           /bin/ls
	//                                                                                                               |
	//                                                                                                            /bin/date
	//                                                                                                               |
	//                                                                                                           /bin/passwd
	//                                                                                                               |
	//                                                                                                           /bin/bpftool
	//                                                                                                               |
	//                                                                                                           /bin/du
	{
		name: "exec/12_bis",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/apache",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/ls",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/date",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/du",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/webserver",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/apache",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 3,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/wc",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 4,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/id",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 5,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 6,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/date",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 7,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/passwd",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 8,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bpftool",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 9,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/du",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 10,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/du",
				},
			},
		},
		wantNewEntry: true,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/apache",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/ls",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/date",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/du",
												},
											},
										},
									},
								},
							},
						},
					},
				},
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/apache",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/wc",
												},
											},
											Children: []*ProcessNode{
												{
													Process: model.Process{
														FileEvent: model.FileEvent{
															PathnameStr: "/bin/id",
														},
													},
													Children: []*ProcessNode{
														{
															Process: model.Process{
																FileEvent: model.FileEvent{
																	PathnameStr: "/bin/ls",
																},
															},
															Children: []*ProcessNode{
																{
																	Process: model.Process{
																		FileEvent: model.FileEvent{
																			PathnameStr: "/bin/date",
																		},
																	},
																	Children: []*ProcessNode{
																		{
																			Process: model.Process{
																				FileEvent: model.FileEvent{
																					PathnameStr: "/bin/passwd",
																				},
																			},
																			Children: []*ProcessNode{
																				{
																					Process: model.Process{
																						FileEvent: model.FileEvent{
																							PathnameStr: "/bin/bpftool",
																						},
																					},
																					Children: []*ProcessNode{
																						{
																							Process: model.Process{
																								FileEvent: model.FileEvent{
																									PathnameStr: "/bin/du",
																								},
																							},
																						},
																					},
																				},
																			},
																		},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/13
	// ---------------
	//
	//      /bin/webserver      +          systemd                              ==>>              /bin/bash
	//          |                            |- /bin/bash -> /bin/webserver                           | (exec)
	//      /bin/ls                          |- /bin/wc                                         /bin/webserver
	//          | (exec)                                                                              |
	//       /bin/wc                                                                               /bin/ls
	//                                                                                                | (exec)
	//                                                                                             /bin/wc
	{
		name: "exec/13",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/webserver",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/ls",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/wc",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
			{
				IsExecChild: true,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/webserver",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 3,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: true,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								IsExecChild: true,
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/ls",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												IsExecChild: true,
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/wc",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/14
	// ---------------
	//
	//      /bin/webserver      +          systemd                                     ==>>              /bin/bash
	//          | (exec)                     |- /bin/bash -> /bin/apache                                    | (exec)
	//      /bin/apache                      |- /bin/ls                                               /bin/webserver
	//          |                                                                                           | (exec)
	//       /bin/wc                                                                                     /bin/apache------
	//                                                                                                      |            |
	//                                                                                                   /bin/wc       /bin/ls
	{
		name: "exec/14",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/webserver",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								IsExecChild: true,
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/apache",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/wc",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
			{
				IsExecChild: true,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/apache",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 3,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: true,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								IsExecChild: true,
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/apache",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/wc",
												},
											},
										},
										{
											Process: model.Process{
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/ls",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/15
	// ---------------
	//
	//      /bin/webserver      +          systemd                                             ==>>              /bin/bash
	//          | (exec)                     |- /bin/bash -> /bin/du -> /bin/apache                                 | (exec)
	//      /bin/date                        |- /bin/ls                                                          /bin/du
	//          | (exec)                                                                                            | (exec)
	//      /bin/apache                                                                                       /bin/webserver
	//          |                                                                                                   | (exec)
	//       /bin/wc                                                                                            /bin/date
	//                                                                                                              | (exec)
	//                                                                                                          /bin/apache------
	//                                                                                                              |            |
	//                                                                                                          /bin/wc       /bin/ls
	{
		name: "exec/15",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/webserver",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								IsExecChild: true,
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/date",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/apache",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/wc",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
			{
				IsExecChild: true,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/du",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
			{
				IsExecChild: true,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/apache",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 3,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 4,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: true,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								IsExecChild: true,
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/du",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/webserver",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												IsExecChild: true,
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/date",
												},
											},
											Children: []*ProcessNode{
												{
													Process: model.Process{
														IsExecChild: true,
														FileEvent: model.FileEvent{
															PathnameStr: "/bin/apache",
														},
													},
													Children: []*ProcessNode{
														{
															Process: model.Process{
																FileEvent: model.FileEvent{
																	PathnameStr: "/bin/wc",
																},
															},
														},
														{
															Process: model.Process{
																FileEvent: model.FileEvent{
																	PathnameStr: "/bin/ls",
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/16
	// ---------------
	//
	//      /bin/bash          +          systemd                 ==>>               /bin/bash
	//          |                            |- /bin/bash                               |
	//      /bin/webserver1                  |- /bin/webserver3                   /bin/webserver1
	//          | (exec)                     |- /bin/ls                                 | (exec)
	//     /bin/webserver2----------         |- /bin/date                         /bin/webserver2---------
	//          | (exec)           |                                                    | (exec)         |
	//     /bin/webserver3      /bin/id                                           /bin/webserver3      /bin/id
	//          |                                                                       |
	//     /bin/webserver4                                                        /bin/webserver4
	//          | (exec)                                                                | (exec)
	//       /bin/ls---------------                                                  /bin/ls----------------------------
	//          |                 |                                                     |                |             |
	//       /bin/wc           /bin/id                                               /bin/wc          /bin/id       /bin/date
	{
		name: "exec/16",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver1",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/webserver2",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/id",
												},
											},
										},
										{
											Process: model.Process{
												IsExecChild: true,
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/webserver3",
												},
											},
											Children: []*ProcessNode{
												{
													Process: model.Process{
														FileEvent: model.FileEvent{
															PathnameStr: "/bin/webserver4",
														},
													},
													Children: []*ProcessNode{
														{
															Process: model.Process{
																IsExecChild: true,
																FileEvent: model.FileEvent{
																	PathnameStr: "/bin/ls",
																},
															},
															Children: []*ProcessNode{
																{
																	Process: model.Process{
																		FileEvent: model.FileEvent{
																			PathnameStr: "/bin/id",
																		},
																	},
																},
																{
																	Process: model.Process{
																		FileEvent: model.FileEvent{
																			PathnameStr: "/bin/wc",
																		},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/webserver3",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 3,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/date",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 4,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/date",
				},
			},
		},
		wantNewEntry: true,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver1",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/webserver2",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/id",
												},
											},
										},
										{
											Process: model.Process{
												IsExecChild: true,
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/webserver3",
												},
											},
											Children: []*ProcessNode{
												{
													Process: model.Process{
														FileEvent: model.FileEvent{
															PathnameStr: "/bin/webserver4",
														},
													},
													Children: []*ProcessNode{
														{
															Process: model.Process{
																IsExecChild: true,
																FileEvent: model.FileEvent{
																	PathnameStr: "/bin/ls",
																},
															},
															Children: []*ProcessNode{
																{
																	Process: model.Process{
																		FileEvent: model.FileEvent{
																			PathnameStr: "/bin/id",
																		},
																	},
																},
																{
																	Process: model.Process{
																		FileEvent: model.FileEvent{
																			PathnameStr: "/bin/wc",
																		},
																	},
																},
																{
																	Process: model.Process{
																		FileEvent: model.FileEvent{
																			PathnameStr: "/bin/date",
																		},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/17
	// ---------------
	//
	//     /bin/bash -----------------           +          systemd                                ==>>             /bin/bash
	//          |                    |                         |- /bin/bash                                             |
	//     /bin/webserver1      /bin/apache                    |- /bin/apache -> /bin/webserver1                   /bin/apache
	//                                                                                                                  | (exec)
	//                                                                                                           /bin/webserver1
	//
	//
	{
		name: "exec/17",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver1",
								},
							},
						},
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/apache",
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/apache",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
			{
				IsExecChild: true,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/webserver1",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 3,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/webserver1",
				},
			},
		},
		wantNewEntry: false,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/apache",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/webserver1",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/18
	// ---------------
	//
	//     /bin/bash        /bin/apache          +          systemd                                ==>>             /bin/bash -----------
	//          |                                              |- /bin/bash -> /bin/apache                              |               | (exec)
	//     /bin/webserver1                                                                                        /bin/webserver1    /bin/apache
	//
	//
	{
		name: "exec/18",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver1",
								},
							},
						},
					},
				},
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/apache",
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
			{
				IsExecChild: true,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/apache",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/apache",
				},
			},
		},
		wantNewEntry: false,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver1",
								},
							},
						},
						{
							Process: model.Process{
								IsExecChild: true,
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/apache",
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/19
	// ---------------
	//
	//     /bin/bash -----------------           +          systemd                                                              ==>>             /bin/bash
	//          |                    |                         |- /bin/bash                                                                           |
	//     /bin/webserver2      /bin/apache                    |- /bin/apache -> /bin/webserver1 -> /bin/webserver3                              /bin/apache
	//          | (exec)                                                                                                                              | (exec)
	//    /bin/webserver3                                                                                                                      /bin/webserver1
	//                                                                                                                                                | (exec)
	//                                                                                                                                         /bin/webserver2
	//                                                                                                                                                | (exec)
	//                                                                                                                                         /bin/webserver3
	//
	{
		name: "exec/19",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver2",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/webserver3",
										},
									},
								},
							},
						},
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/apache",
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/apache",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
			{
				IsExecChild: true,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/webserver1",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 3,
						},
					},
				},
			},
			{
				IsExecChild: true,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/webserver3",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 4,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/webserver3",
				},
			},
		},
		wantNewEntry: true,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/apache",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/webserver1",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												IsExecChild: true,
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/webserver2",
												},
											},
											Children: []*ProcessNode{
												{
													Process: model.Process{
														IsExecChild: true,
														FileEvent: model.FileEvent{
															PathnameStr: "/bin/webserver3",
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/20
	// ---------------
	//
	//      /bin/1--------         +       systemd                 ==>>   /bin/1 -------     /bin/4
	//         |         |                 |- /bin/4                         |         |        |
	//      /bin/2    /bin/3               |- /bin/2                       /bin/2    /bin/3   /bin/2
	{
		name: "exec/20",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/1",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/2",
								},
							},
						},
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/3",
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/4",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 4,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/2",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/2",
				},
			},
		},
		wantNewEntry: true,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/1",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/2",
								},
							},
						},
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/3",
								},
							},
						},
					},
				},
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/4",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/2",
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/21
	// ---------------
	//
	//       bin/1------------           /bin/4          +        systemd                   ==>>        /bin/4
	//         | (exec)      | (exec)                                |- /bin/4 -> /bin/2                   | (exec)
	//      /bin/2         /bin/3                                                                       /bin/1 -------------
	//                                                                                                    | (exec)         | (exec)
	//                                                                                                  /bin/2          /bin/3
	//
	{
		name: "exec/21",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						IsExecChild: false,
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/1",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								IsExecChild: true,
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/2",
								},
							},
						},
						{
							Process: model.Process{
								IsExecChild: true,
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/3",
								},
							},
						},
					},
				},
				{
					Process: model.Process{
						IsExecChild: false,
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/4",
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				IsExecChild: false,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/4",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 4,
						},
					},
				},
			},
			{
				IsExecChild: true,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/2",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/2",
				},
			},
		},
		wantNewEntry: false,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						IsExecChild: false,
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/4",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								IsExecChild: true,
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/1",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/2",
										},
									},
								},
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/3",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/22
	// ---------------
	//      /bin/0                          +       systemd                      ==>>         /bin/0                          /bin/4
	//         |                                       |- /bin/4 -> /bin/2                       |                               | (exec)
	//      /bin/1---------------                                                             /bin/1 -----------              /bin/2
	//         | (exec)         | (exec)                                                         | (exec)      | (exec)
	//      /bin/2            /bin/3                                                          /bin/2         /bin/3
	//
	{
		name: "exec/22",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						IsExecChild: false,
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/0",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								IsExecChild: false,
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/1",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/2",
										},
									},
								},
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/3",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				IsExecChild: false,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/4",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 4,
						},
					},
				},
			},
			{
				IsExecChild: true,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/2",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/2",
				},
			},
		},
		wantNewEntry: true,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						IsExecChild: false,
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/0",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								IsExecChild: false,
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/1",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/2",
										},
									},
								},
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/3",
										},
									},
								},
							},
						},
					},
				},
				{
					Process: model.Process{
						IsExecChild: false,
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/4",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								IsExecChild: true,
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/2",
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/23
	// ---------------
	//        dash---------            +        systemd             ==>    dash
	//         |          |                        |                        |
	//        bash     ddtrace                    dash                     bash------------
	//         |         | (exec)                  |                        |             | (exec)
	//       python    uwsgi                      bash                   python        ddtrace
	//                                             |                                      | (exec)
	//                                           ddtrace                                tools
	//                                             | (exec)                               | (exec)
	//                                           tools                                  utils
	//                                             | (exec)                               | (exec)
	//                                           utils                                  uwsgi
	//                                             | (exec)
	//                                           uwsgi
	//
	{
		name: "exec/23",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "dash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "bash",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										FileEvent: model.FileEvent{
											PathnameStr: "python",
										},
									},
								},
							},
						},
						{
							Process: model.Process{
								IsExecChild: false,
								FileEvent: model.FileEvent{
									PathnameStr: "ddtrace",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "uwsgi",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				IsExecChild: false,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "dash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
			{
				IsExecChild: false,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
			{
				IsExecChild: false,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "ddtrace",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 3,
						},
					},
				},
			},
			{
				IsExecChild: true,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "tools",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 4,
						},
					},
				},
			},
			{
				IsExecChild: true,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "utils",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 5,
						},
					},
				},
			},
			{
				IsExecChild: true,
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "uwsgi",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 6,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "uwsgi",
				},
			},
		},
		wantNewEntry: true,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "dash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "bash",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										FileEvent: model.FileEvent{
											PathnameStr: "python",
										},
									},
								},
								{
									Process: model.Process{
										IsExecChild: true,
										FileEvent: model.FileEvent{
											PathnameStr: "ddtrace",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												IsExecChild: true,
												FileEvent: model.FileEvent{
													PathnameStr: "tools",
												},
											},
											Children: []*ProcessNode{
												{
													Process: model.Process{
														IsExecChild: true,
														FileEvent: model.FileEvent{
															PathnameStr: "utils",
														},
													},
													Children: []*ProcessNode{
														{
															Process: model.Process{
																IsExecChild: true,
																FileEvent: model.FileEvent{
																	PathnameStr: "uwsgi",
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},
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

		_, newEntry, err := tree.CreateProcessNode(event.ProcessCacheEntry, Runtime, false, nil)
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
							PathnameStr: "/tmp/*/script.sh",
						},
					},
				},
			},
		}

		_, newEntry, err = tree.CreateProcessNode(event.ProcessCacheEntry, Runtime, false, nil)
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

		_, newEntry, err := tree.CreateProcessNode(event.ProcessCacheEntry, Runtime, false, nil)
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

		_, newEntry, err = tree.CreateProcessNode(event.ProcessCacheEntry, Runtime, true, nil)
		assert.NoError(t, err)
		assert.True(t, newEntry)
		assertTreeEqual(t, wanted, tree)
	})
}
