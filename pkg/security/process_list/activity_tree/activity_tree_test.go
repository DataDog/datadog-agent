// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	processlist "github.com/DataDog/datadog-agent/pkg/security/process_list"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

type testIteration struct {
	testName          string
	resetActivityTree bool

	// input
	parentProcessPath     string
	processPath           string
	granpaInsideContainer bool
	completeLineage       bool
	differentContainerID  bool
	fileLessParent        bool
	fileLess              bool
	// setCookie             bool
	// setCookieParent       bool

	// output
	resultInserted bool
	resultErr      error
	resultTree     map[string][]string
}

var (
	defaultContainerID  string = "424242424242424242424242424242424242424242424242424242424242424"
	defaultContainerID2 string = "515151515151515151515151515151515151515151515151515151515151515"
)

type treeType int

const (
	dumpTree treeType = iota
	profileTree
)

func (tt treeType) String() string {
	if tt == dumpTree {
		return "dump"
	}
	return "profile"
}

func matchResultTree(pl *processlist.ProcessList, toMatch map[string][]string) bool {
	// fmt.Printf("\n\nmatchResultTree:\n")
	// pl.Debug(os.Stdout)
	// fmt.Printf("VS:\n%+v\n", toMatch)

	rootNodes := pl.GetChildren()
	if rootNodes == nil {
		return false
	}

	if len(*rootNodes) != len(toMatch) {
		return false
	}

	for _, node := range *rootNodes {
		childrens, ok := toMatch[node.CurrentExec.FileEvent.PathnameStr]
		if !ok {
			return false
		} else if len(childrens) != len(node.Children) {
			return false
		}
		for _, child := range node.Children {
			if !slices.Contains(childrens, child.CurrentExec.FileEvent.PathnameStr) {
				return false
			} else if len(child.Children) > 0 {
				return false
			}
		}
	}
	return true
}

func craftFakeEvent(test *testIteration) *model.Event {
	e := model.NewFakeEvent()
	e.Type = uint32(model.ExecEventType)
	e.ProcessContext = &model.ProcessContext{}
	e.ProcessContext.PPid = 41
	e.ProcessContext.Pid = 42
	e.ProcessContext.ForkTime = time.Now()
	e.ProcessContext.ContainerID = containerutils.ContainerID(defaultContainerID)
	e.ProcessContext.FileEvent.PathnameStr = test.processPath
	e.ProcessContext.FileEvent.PathnameStr = test.processPath
	e.ProcessContext.Argv0 = filepath.Base(test.processPath)
	// e.ProcessContext.FileEvent.Inode = 42
	// if !test.fileLess {
	// 	e.ProcessContext.FileEvent.MountID = 42
	// }
	e.ProcessContext.Args = "foo"

	// if test.setCookie {
	// 	process.Cookie = 42
	// }

	// setting process ancestor
	e.ProcessContext.Ancestor = model.NewPlaceholderProcessCacheEntry(41, 41, false)
	e.ProcessContext.Ancestor.ContainerID = containerutils.ContainerID(defaultContainerID)
	e.ProcessContext.Ancestor.FileEvent.PathnameStr = test.parentProcessPath
	e.ProcessContext.Ancestor.FileEvent.BasenameStr = filepath.Base(test.parentProcessPath)
	e.ProcessContext.Ancestor.Argv0 = filepath.Base(test.parentProcessPath)
	// make the same inode/mountid if the parent and the child have the same path
	// id := 41
	// if test.processPath == test.parentProcessPath {
	// 	id = 42
	// }
	// e.ProcessContext.Ancestor.FileEvent.Inode = uint64(id)
	// if !test.fileLessParent {
	// 	e.ProcessContext.Ancestor.FileEvent.MountID = uint32(id)
	// }
	e.ProcessContext.Ancestor.Args = "bar"
	// if test.setCookieParent {
	// 	process.Ancestor.Cookie = 41
	// }

	// setting process granpa
	if test.completeLineage {
		e.ProcessContext.Ancestor.PPid = 1
		e.ProcessContext.Ancestor.Ancestor = model.NewPlaceholderProcessCacheEntry(1, 1, false)
	} else {
		e.ProcessContext.Ancestor.PPid = 40
		e.ProcessContext.Ancestor.Ancestor = model.NewPlaceholderProcessCacheEntry(40, 40, false)
	}
	e.ProcessContext.Ancestor.Ancestor.FileEvent.PathnameStr = "/usr/bin/systemd"
	e.ProcessContext.Ancestor.Ancestor.FileEvent.BasenameStr = "systemd"
	if test.granpaInsideContainer {
		e.ProcessContext.Ancestor.Ancestor.ContainerID = containerutils.ContainerID(defaultContainerID)
	}
	// e.ProcessContext.Ancestor.Ancestor.FileEvent.Inode = 40
	// e.ProcessContext.Ancestor.Ancestor.FileEvent.MountID = 40
	e.ProcessContext.Ancestor.Ancestor.Args = "start"

	return e
}

func TestActivityTree_CreateProcessNode(t *testing.T) {
	tests := []testIteration{

		// // check process with broken lineage (parent with pid != 1 && containerID != "")
		// {
		// 	testName:              "broken_lineage",
		// 	resetActivityTree:     true,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/bar",
		// 	completeLineage:       false,
		// 	resultInserted:  false,
		// 	resultErr:             activity_tree.ErrBrokenLineage,
		// 	resultTree:            map[string][]string{},
		// },

		// // check that a process with a different containerID will not be inserted
		// {
		// 	testName:              "containerID-mismatch",
		// 	resetActivityTree:     true,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/bar",
		// 	completeLineage:       true,
		// 	differentContainerID:  true,
		// 	resultInserted:  false,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{},
		// },

		// check a simple child/parent insertion without any cookies
		{
			testName:             "simple-insert-without-cookies",
			resetActivityTree:    true,
			parentProcessPath:    "/bin/foo",
			processPath:          "/bin/bar",
			completeLineage:      true,
			differentContainerID: false,
			resultInserted:       true,
			resultErr:            nil,
			resultTree:           map[string][]string{"/bin/foo": {"/bin/bar"}},
		},
		// // make the same insert as previous one with node cookie
		// {
		// 	testName:              "simple-insert-twice-cookie-1",
		// 	resetActivityTree:     false,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/bar",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	setCookie:             true,
		// 	resultInserted:  false,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		// },
		// // make the same insert as previous one with parent cookies
		// {
		// 	testName:              "simple-insert-twice-parent-cookie-1",
		// 	resetActivityTree:     false,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/bar",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	setCookieParent:       true,
		// 	resultInserted:  false,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		// },
		// // make the same insert as previous one with node and parent cookies
		// {
		// 	testName:              "simple-insert-twice-node-and-parent-cookies-1",
		// 	resetActivityTree:     false,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/bar",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	setCookie:             true,
		// 	setCookieParent:       true,
		// 	resultInserted:  false,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		// },
		// // add a child to existing parent node with parent cookie
		// {
		// 	testName:              "insert-new-child-1",
		// 	resetActivityTree:     false,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/baz",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	setCookieParent:       true,
		// 	resultInserted:  true,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/foo": {"/bin/bar", "/bin/baz"}},
		// },
		// // add a child to existing parent node without parent cookie
		// {
		// 	testName:              "insert-new-child-1",
		// 	resetActivityTree:     false,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/baz2",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	resultInserted:  true,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/foo": {"/bin/bar", "/bin/baz", "/bin/baz2"}},
		// },

		// // check a simple child/parent insertion with node cookie
		// {
		// 	testName:              "simple-insert-with-node-cookie",
		// 	resetActivityTree:     true,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/bar",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	setCookie:             true,
		// 	resultInserted:  true,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		// },
		// // make the same insert as previous one with node cookie
		// {
		// 	testName:              "simple-insert-twice-cookie-2",
		// 	resetActivityTree:     false,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/bar",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	setCookie:             true,
		// 	resultInserted:  false,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		// },
		// // make the same insert as previous one with parent cookies
		// {
		// 	testName:              "simple-insert-twice-parent-cookie-2",
		// 	resetActivityTree:     false,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/bar",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	setCookieParent:       true,
		// 	resultInserted:  false,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		// },
		// // make the same insert as previous one with node and parent cookies
		// {
		// 	testName:              "simple-insert-twice-node-and-parent-cookies-2",
		// 	resetActivityTree:     false,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/bar",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	setCookie:             true,
		// 	setCookieParent:       true,
		// 	resultInserted:  false,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		// },
		// // add a child to existing parent node with parent cookie
		// {
		// 	testName:              "insert-new-child-2",
		// 	resetActivityTree:     false,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/baz",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	setCookieParent:       true,
		// 	resultInserted:  true,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/foo": {"/bin/bar", "/bin/baz"}},
		// },
		// // add a child to existing parent node without parent cookie
		// {
		// 	testName:              "insert-new-child-2",
		// 	resetActivityTree:     false,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/baz2",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	resultInserted:  true,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/foo": {"/bin/bar", "/bin/baz", "/bin/baz2"}},
		// },

		// // check a simple child/parent insertion with parent cookie
		// {
		// 	testName:              "simple-insert-with-parent-cookie",
		// 	resetActivityTree:     true,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/bar",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	setCookieParent:       true,
		// 	resultInserted:  true,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		// },
		// // make the same insert as previous one with node cookie
		// {
		// 	testName:              "simple-insert-twice-cookie-3",
		// 	resetActivityTree:     false,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/bar",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	setCookie:             true,
		// 	resultInserted:  false,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		// },
		// // make the same insert as previous one with parent cookies
		// {
		// 	testName:              "simple-insert-twice-parent-cookie-3",
		// 	resetActivityTree:     false,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/bar",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	setCookieParent:       true,
		// 	resultInserted:  false,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		// },
		// // make the same insert as previous one with node and parent cookies
		// {
		// 	testName:              "simple-insert-twice-node-and-parent-cookies-3",
		// 	resetActivityTree:     false,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/bar",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	setCookie:             true,
		// 	setCookieParent:       true,
		// 	resultInserted:  false,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		// },
		// // add a child to existing parent node with parent cookie
		// {
		// 	testName:              "insert-new-child-3",
		// 	resetActivityTree:     false,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/baz",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	setCookieParent:       true,
		// 	resultInserted:  true,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/foo": {"/bin/bar", "/bin/baz"}},
		// },
		// // add a child to existing parent node without parent cookie
		// {
		// 	testName:              "insert-new-child-3",
		// 	resetActivityTree:     false,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/baz2",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	resultInserted:  true,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/foo": {"/bin/bar", "/bin/baz", "/bin/baz2"}},
		// },

		// // check a simple child/parent insertion with node and parent cookies
		// {
		// 	testName:              "simple-insert-with-parent-and-node-cookies",
		// 	resetActivityTree:     true,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/bar",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	setCookie:             true,
		// 	setCookieParent:       true,
		// 	resultInserted:  true,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		// },
		// // make the same insert as previous one with node cookie
		// {
		// 	testName:              "simple-insert-twice-cookie-4",
		// 	resetActivityTree:     false,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/bar",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	setCookie:             true,
		// 	resultInserted:  false,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		// },
		// // make the same insert as previous one with parent cookies
		// {
		// 	testName:              "simple-insert-twice-parent-cookie-4",
		// 	resetActivityTree:     false,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/bar",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	setCookieParent:       true,
		// 	resultInserted:  false,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		// },
		// // make the same insert as previous one with node and parent cookies
		// {
		// 	testName:              "simple-insert-twice-node-and-parent-cookies-4",
		// 	resetActivityTree:     false,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/bar",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	setCookie:             true,
		// 	setCookieParent:       true,
		// 	resultInserted:  false,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		// },
		// // add a child to existing parent node with parent cookie
		// {
		// 	testName:              "insert-new-child-4",
		// 	resetActivityTree:     false,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/baz",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	setCookieParent:       true,
		// 	resultInserted:  true,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/foo": {"/bin/bar", "/bin/baz"}},
		// },
		// // add a child to existing parent node without parent cookie
		// {
		// 	testName:              "insert-new-child-4",
		// 	resetActivityTree:     false,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/baz2",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	resultInserted:  true,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/foo": {"/bin/bar", "/bin/baz", "/bin/baz2"}},
		// },

		// // try to insert a fileless root node
		// {
		// 	testName:              "try-insert-fileless-root-node",
		// 	resetActivityTree:     true,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/bar",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	fileLessParent:        true,
		// 	resultInserted:  true,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		// },

		// // try to insert a fileless node
		// {
		// 	testName:              "try-insert-fileless-node",
		// 	resetActivityTree:     true,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/bar",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	fileLess:              true,
		// 	resultInserted:  true,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		// },

		// // try to insert the granpa node inside the container
		// {
		// 	testName:              "try-insert-init-in-container",
		// 	resetActivityTree:     true,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/bar",
		// 	granpaInsideContainer: true,
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	resultInserted:  true,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		// },

		// // insert a runc node
		// {
		// 	testName:              "insert-runc-node",
		// 	resetActivityTree:     true,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/runc",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	resultInserted:  true,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/foo": {"/bin/runc"}},
		// },

		// // try insert a runc node and parent node
		// {
		// 	testName:              "insert-runc-node-and-root",
		// 	resetActivityTree:     true,
		// 	parentProcessPath:     "/bin/runc",
		// 	processPath:           "/bin/runc",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	resultInserted:  false,
		// 	resultErr:             activity_tree.ErrNotValidRootNode,
		// 	resultTree:            map[string][]string{},
		// },

		// // try insert a runc node and parent node, will instead insert child node as root
		// {
		// 	testName:              "insert-runc-root",
		// 	resetActivityTree:     true,
		// 	parentProcessPath:     "/bin/runc",
		// 	processPath:           "/bin/bar",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	resultInserted:  true,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/bar": {}},
		// },

		// // try insert fileless runc as root node
		// {
		// 	testName:              "insert-runc-root",
		// 	resetActivityTree:     true,
		// 	parentProcessPath:     "/bin/runc",
		// 	processPath:           "/bin/bar",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	fileLessParent:        true,
		// 	resultInserted:  true,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/bar": {}},
		// },

		// // try insert a fileless node with a runc parent
		// {
		// 	testName:              "insert-fileless-node-with-runc-parent",
		// 	resetActivityTree:     true,
		// 	parentProcessPath:     "/bin/runc",
		// 	processPath:           "/bin/bar",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	fileLess:              true,
		// 	resultInserted:  false,
		// 	resultErr:             activity_tree.ErrNotValidRootNode,
		// 	resultTree:            map[string][]string{},
		// },

		// // try insert root and child with same exec
		// {
		// 	testName:              "insert-exec-exec",
		// 	resetActivityTree:     true,
		// 	parentProcessPath:     "/bin/foo",
		// 	processPath:           "/bin/foo",
		// 	completeLineage:       true,
		// 	differentContainerID:  false,
		// 	fileLess:              true,
		// 	resultInserted:  true,
		// 	resultErr:             nil,
		// 	resultTree:            map[string][]string{"/bin/foo": {}},
		// },
	}

	at := NewActivityTree(nil, false, 3)
	var pl *processlist.ProcessList = nil

	for _, ti := range tests {
		testName := ti.testName
		t.Run(testName, func(t *testing.T) {
			if ti.resetActivityTree || pl == nil {
				// contID := defaultContainerID
				// if ti.differentContainerID {
				// 	contID = defaultContainerID2
				// }

				pl = processlist.NewProcessList(cgroupModel.WorkloadSelector{Image: "image", Tag: "tag"},
					[]model.EventType{model.ExecEventType, model.ForkEventType, model.FileOpenEventType}, at /* ,nil  */, nil, nil)
			}

			event := craftFakeEvent(&ti)
			inserted, err := pl.Insert(event, true, "tag")
			assert.Equal(t, ti.resultErr, err)
			assert.Equal(t, ti.resultInserted, inserted)
			if !matchResultTree(pl, ti.resultTree) {
				t.Error("result tree did not match")
			}
		})
	}
}
