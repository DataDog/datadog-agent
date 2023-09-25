// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package securityprofiletests holds securityprofiletests related files
package securityprofiletests

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/exp/slices"

	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/dump"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
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
	setCookie             bool
	setCookieParent       bool

	// output
	resultNodeShouldBeNil bool
	resultNewProcessNode  bool
	resultErr             error
	resultTree            map[string][]string
}

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

func matchResultTree(at *activity_tree.ActivityTree, toMatch map[string][]string) bool {
	if len(at.ProcessNodes) != len(toMatch) {
		return false
	}

	for _, node := range at.ProcessNodes {
		childrens, ok := toMatch[node.Process.FileEvent.PathnameStr]
		if !ok {
			return false
		} else if len(childrens) != len(node.Children) {
			return false
		}
		for _, child := range node.Children {
			if !slices.Contains(childrens, child.Process.FileEvent.PathnameStr) {
				return false
			} else if len(child.Children) > 0 {
				return false
			}
		}
	}
	return true
}

func craftFakeProcess(containerID string, test *testIteration) *model.ProcessCacheEntry {
	// setting process
	process := model.NewPlaceholderProcessCacheEntry(42, 42, false)
	process.ContainerID = containerID
	process.FileEvent.PathnameStr = test.processPath
	process.FileEvent.BasenameStr = filepath.Base(test.processPath)
	process.Argv0 = filepath.Base(test.processPath)
	process.FileEvent.Inode = 42
	if !test.fileLess {
		process.FileEvent.MountID = 42
	}
	process.Args = "foo"
	if test.setCookie {
		process.Cookie = 42
	}

	// setting process ancestor
	process.Ancestor = model.NewPlaceholderProcessCacheEntry(41, 41, false)
	process.Ancestor.ContainerID = containerID
	process.Ancestor.FileEvent.PathnameStr = test.parentProcessPath
	process.Ancestor.FileEvent.BasenameStr = filepath.Base(test.parentProcessPath)
	process.Ancestor.Argv0 = filepath.Base(test.parentProcessPath)
	// make the same inode/mountid if the parent and the child have the same path
	id := 41
	if test.processPath == test.parentProcessPath {
		id = 42
	}
	process.Ancestor.FileEvent.Inode = uint64(id)
	if !test.fileLessParent {
		process.Ancestor.FileEvent.MountID = uint32(id)
	}
	process.Ancestor.Args = "bar"
	if test.setCookieParent {
		process.Ancestor.Cookie = 41
	}

	// setting process granpa
	if test.completeLineage {
		process.Ancestor.Ancestor = model.NewPlaceholderProcessCacheEntry(1, 1, false)
	} else {
		process.Ancestor.Ancestor = model.NewPlaceholderProcessCacheEntry(40, 40, false)
	}
	process.Ancestor.Ancestor.FileEvent.PathnameStr = "/usr/bin/systemd"
	process.Ancestor.Ancestor.FileEvent.BasenameStr = "systemd"
	if test.granpaInsideContainer {
		process.Ancestor.Ancestor.ContainerID = containerID
	}
	process.Ancestor.Ancestor.FileEvent.Inode = 40
	process.Ancestor.Ancestor.FileEvent.MountID = 40
	process.Ancestor.Ancestor.Args = "start"
	return process
}

func TestActivityTree_CreateProcessNode(t *testing.T) {
	defaultContainerID := "424242424242424242424242424242424242424242424242424242424242424"
	defaultContainerID2 := "515151515151515151515151515151515151515151515151515151515151515"

	tests := []testIteration{

		// check process with broken lineage (parent with pid != 1)
		{
			testName:              "broken_lineage",
			resetActivityTree:     true,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/bar",
			completeLineage:       false,
			resultNodeShouldBeNil: true,
			resultNewProcessNode:  false,
			resultErr:             activity_tree.ErrBrokenLineage,
			resultTree:            map[string][]string{},
		},

		// check that a process with a different containerID will not be inserted
		{
			testName:              "containerID-mismatch",
			resetActivityTree:     true,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/bar",
			completeLineage:       true,
			differentContainerID:  true,
			resultNodeShouldBeNil: true,
			resultNewProcessNode:  false,
			resultErr:             nil,
			resultTree:            map[string][]string{},
		},

		// check a simple child/parent insertion without any cookies
		{
			testName:              "simple-insert-without-cookies",
			resetActivityTree:     true,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/bar",
			completeLineage:       true,
			differentContainerID:  false,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  true,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		},
		// make the same insert as previous one with node cookie
		{
			testName:              "simple-insert-twice-cookie-1",
			resetActivityTree:     false,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/bar",
			completeLineage:       true,
			differentContainerID:  false,
			setCookie:             true,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  false,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		},
		// make the same insert as previous one with parent cookies
		{
			testName:              "simple-insert-twice-parent-cookie-1",
			resetActivityTree:     false,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/bar",
			completeLineage:       true,
			differentContainerID:  false,
			setCookieParent:       true,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  false,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		},
		// make the same insert as previous one with node and parent cookies
		{
			testName:              "simple-insert-twice-node-and-parent-cookies-1",
			resetActivityTree:     false,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/bar",
			completeLineage:       true,
			differentContainerID:  false,
			setCookie:             true,
			setCookieParent:       true,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  false,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		},
		// add a child to existing parent node with parent cookie
		{
			testName:              "insert-new-child-1",
			resetActivityTree:     false,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/baz",
			completeLineage:       true,
			differentContainerID:  false,
			setCookieParent:       true,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  true,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/foo": {"/bin/bar", "/bin/baz"}},
		},
		// add a child to existing parent node without parent cookie
		{
			testName:              "insert-new-child-1",
			resetActivityTree:     false,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/baz2",
			completeLineage:       true,
			differentContainerID:  false,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  true,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/foo": {"/bin/bar", "/bin/baz", "/bin/baz2"}},
		},

		// check a simple child/parent insertion with node cookie
		{
			testName:              "simple-insert-with-node-cookie",
			resetActivityTree:     true,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/bar",
			completeLineage:       true,
			differentContainerID:  false,
			setCookie:             true,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  true,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		},
		// make the same insert as previous one with node cookie
		{
			testName:              "simple-insert-twice-cookie-2",
			resetActivityTree:     false,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/bar",
			completeLineage:       true,
			differentContainerID:  false,
			setCookie:             true,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  false,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		},
		// make the same insert as previous one with parent cookies
		{
			testName:              "simple-insert-twice-parent-cookie-2",
			resetActivityTree:     false,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/bar",
			completeLineage:       true,
			differentContainerID:  false,
			setCookieParent:       true,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  false,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		},
		// make the same insert as previous one with node and parent cookies
		{
			testName:              "simple-insert-twice-node-and-parent-cookies-2",
			resetActivityTree:     false,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/bar",
			completeLineage:       true,
			differentContainerID:  false,
			setCookie:             true,
			setCookieParent:       true,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  false,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		},
		// add a child to existing parent node with parent cookie
		{
			testName:              "insert-new-child-2",
			resetActivityTree:     false,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/baz",
			completeLineage:       true,
			differentContainerID:  false,
			setCookieParent:       true,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  true,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/foo": {"/bin/bar", "/bin/baz"}},
		},
		// add a child to existing parent node without parent cookie
		{
			testName:              "insert-new-child-2",
			resetActivityTree:     false,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/baz2",
			completeLineage:       true,
			differentContainerID:  false,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  true,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/foo": {"/bin/bar", "/bin/baz", "/bin/baz2"}},
		},

		// check a simple child/parent insertion with parent cookie
		{
			testName:              "simple-insert-with-parent-cookie",
			resetActivityTree:     true,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/bar",
			completeLineage:       true,
			differentContainerID:  false,
			setCookieParent:       true,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  true,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		},
		// make the same insert as previous one with node cookie
		{
			testName:              "simple-insert-twice-cookie-3",
			resetActivityTree:     false,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/bar",
			completeLineage:       true,
			differentContainerID:  false,
			setCookie:             true,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  false,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		},
		// make the same insert as previous one with parent cookies
		{
			testName:              "simple-insert-twice-parent-cookie-3",
			resetActivityTree:     false,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/bar",
			completeLineage:       true,
			differentContainerID:  false,
			setCookieParent:       true,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  false,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		},
		// make the same insert as previous one with node and parent cookies
		{
			testName:              "simple-insert-twice-node-and-parent-cookies-3",
			resetActivityTree:     false,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/bar",
			completeLineage:       true,
			differentContainerID:  false,
			setCookie:             true,
			setCookieParent:       true,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  false,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		},
		// add a child to existing parent node with parent cookie
		{
			testName:              "insert-new-child-3",
			resetActivityTree:     false,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/baz",
			completeLineage:       true,
			differentContainerID:  false,
			setCookieParent:       true,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  true,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/foo": {"/bin/bar", "/bin/baz"}},
		},
		// add a child to existing parent node without parent cookie
		{
			testName:              "insert-new-child-3",
			resetActivityTree:     false,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/baz2",
			completeLineage:       true,
			differentContainerID:  false,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  true,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/foo": {"/bin/bar", "/bin/baz", "/bin/baz2"}},
		},

		// check a simple child/parent insertion with node and parent cookies
		{
			testName:              "simple-insert-with-parent-and-node-cookies",
			resetActivityTree:     true,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/bar",
			completeLineage:       true,
			differentContainerID:  false,
			setCookie:             true,
			setCookieParent:       true,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  true,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		},
		// make the same insert as previous one with node cookie
		{
			testName:              "simple-insert-twice-cookie-4",
			resetActivityTree:     false,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/bar",
			completeLineage:       true,
			differentContainerID:  false,
			setCookie:             true,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  false,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		},
		// make the same insert as previous one with parent cookies
		{
			testName:              "simple-insert-twice-parent-cookie-4",
			resetActivityTree:     false,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/bar",
			completeLineage:       true,
			differentContainerID:  false,
			setCookieParent:       true,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  false,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		},
		// make the same insert as previous one with node and parent cookies
		{
			testName:              "simple-insert-twice-node-and-parent-cookies-4",
			resetActivityTree:     false,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/bar",
			completeLineage:       true,
			differentContainerID:  false,
			setCookie:             true,
			setCookieParent:       true,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  false,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		},
		// add a child to existing parent node with parent cookie
		{
			testName:              "insert-new-child-4",
			resetActivityTree:     false,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/baz",
			completeLineage:       true,
			differentContainerID:  false,
			setCookieParent:       true,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  true,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/foo": {"/bin/bar", "/bin/baz"}},
		},
		// add a child to existing parent node without parent cookie
		{
			testName:              "insert-new-child-4",
			resetActivityTree:     false,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/baz2",
			completeLineage:       true,
			differentContainerID:  false,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  true,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/foo": {"/bin/bar", "/bin/baz", "/bin/baz2"}},
		},

		// try to insert a fileless root node
		{
			testName:              "try-insert-fileless-root-node",
			resetActivityTree:     true,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/bar",
			completeLineage:       true,
			differentContainerID:  false,
			fileLessParent:        true,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  true,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		},

		// try to insert a fileless node
		{
			testName:              "try-insert-fileless-node",
			resetActivityTree:     true,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/bar",
			completeLineage:       true,
			differentContainerID:  false,
			fileLess:              true,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  true,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		},

		// try to insert the granpa node inside the container
		{
			testName:              "try-insert-init-in-container",
			resetActivityTree:     true,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/bar",
			granpaInsideContainer: true,
			completeLineage:       true,
			differentContainerID:  false,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  true,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/foo": {"/bin/bar"}},
		},

		// insert a runc node
		{
			testName:              "insert-runc-node",
			resetActivityTree:     true,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/runc",
			completeLineage:       true,
			differentContainerID:  false,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  true,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/foo": {"/bin/runc"}},
		},

		// try insert a runc node and parent node
		{
			testName:              "insert-runc-node-and-root",
			resetActivityTree:     true,
			parentProcessPath:     "/bin/runc",
			processPath:           "/bin/runc",
			completeLineage:       true,
			differentContainerID:  false,
			resultNodeShouldBeNil: true,
			resultNewProcessNode:  false,
			resultErr:             activity_tree.ErrNotValidRootNode,
			resultTree:            map[string][]string{},
		},

		// try insert a runc node and parent node, will instead insert child node as root
		{
			testName:              "insert-runc-root",
			resetActivityTree:     true,
			parentProcessPath:     "/bin/runc",
			processPath:           "/bin/bar",
			completeLineage:       true,
			differentContainerID:  false,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  true,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/bar": {}},
		},

		// try insert fileless runc as root node
		{
			testName:              "insert-runc-root",
			resetActivityTree:     true,
			parentProcessPath:     "/bin/runc",
			processPath:           "/bin/bar",
			completeLineage:       true,
			differentContainerID:  false,
			fileLessParent:        true,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  true,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/bar": {}},
		},

		// try insert a fileless node with a runc parent
		{
			testName:              "insert-fileless-node-with-runc-parent",
			resetActivityTree:     true,
			parentProcessPath:     "/bin/runc",
			processPath:           "/bin/bar",
			completeLineage:       true,
			differentContainerID:  false,
			fileLess:              true,
			resultNodeShouldBeNil: true,
			resultNewProcessNode:  false,
			resultErr:             activity_tree.ErrNotValidRootNode,
			resultTree:            map[string][]string{},
		},

		// try insert root and child with same exec
		{
			testName:              "insert-exec-exec",
			resetActivityTree:     true,
			parentProcessPath:     "/bin/foo",
			processPath:           "/bin/foo",
			completeLineage:       true,
			differentContainerID:  false,
			fileLess:              true,
			resultNodeShouldBeNil: false,
			resultNewProcessNode:  true,
			resultErr:             nil,
			resultTree:            map[string][]string{"/bin/foo": {}},
		},
	}

	treeSavedStates := map[treeType]map[activity_tree.NodeGenerationType]*activity_tree.ActivityTree{
		profileTree: {
			activity_tree.Unknown:        nil,
			activity_tree.Runtime:        nil,
			activity_tree.Snapshot:       nil,
			activity_tree.ProfileDrift:   nil,
			activity_tree.WorkloadWarmup: nil,
		},
		dumpTree: {
			activity_tree.Unknown:        nil,
			activity_tree.Runtime:        nil,
			activity_tree.Snapshot:       nil,
			activity_tree.ProfileDrift:   nil,
			activity_tree.WorkloadWarmup: nil,
		},
	}
	var at *activity_tree.ActivityTree

	for _, ti := range tests {

		// for each test we run a 3D matrix for tree type (profile or dump), generation type (Unknown, Runtime, Snapshot, ProfileDrift or WorkloadWarmup) and dry-run (with or without)
		for _, tt := range []treeType{profileTree, dumpTree} {
			for _, gentype := range []activity_tree.NodeGenerationType{activity_tree.Unknown, activity_tree.Runtime, activity_tree.Snapshot, activity_tree.ProfileDrift, activity_tree.WorkloadWarmup} {
				for _, dryRun := range []bool{true, false} { // dry-run have to be run first as we may retrieve previous saved state
					testName := ti.testName
					testName += "/" + gentype.String()
					testName += "/" + tt.String()
					if dryRun {
						testName += "-dryrun"
					}

					t.Run(testName, func(t *testing.T) {
						if ti.resetActivityTree {
							contID := defaultContainerID
							if ti.differentContainerID {
								contID = defaultContainerID2
							}

							if tt == dumpTree {
								dump := dump.NewEmptyActivityDump(nil)
								dump.Metadata.ContainerID = contID
								at = dump.ActivityTree
							} else /* profileTree */ {
								profile := profile.NewSecurityProfile(cgroupModel.WorkloadSelector{Image: "image", Tag: "tag"}, []model.EventType{model.ExecEventType, model.DNSEventType})
								at = activity_tree.NewActivityTree(profile, nil, "profile")
								profile.ActivityTree = at
								profile.Instances = append(profile.Instances, &cgroupModel.CacheEntry{
									ContainerContext: model.ContainerContext{ID: contID},
									WorkloadSelector: cgroupModel.WorkloadSelector{Image: "image", Tag: "tag"},
								})
							}
						} else { // retrieve last saved tree state
							at = treeSavedStates[tt][gentype]
						}

						process := craftFakeProcess(defaultContainerID, &ti)

						node, newProcessNode, err := at.CreateProcessNode(process, gentype, dryRun, nil)

						assert.Equal(t, ti.resultErr, err)
						assert.Equal(t, ti.resultNewProcessNode, newProcessNode)
						if dryRun == false { // only check the returned node if dryRun is false
							assert.Equal(t, ti.resultNodeShouldBeNil, node == nil)
							if !matchResultTree(at, ti.resultTree) {
								t.Error("result tree did not match")
							}
						}

						// Save activity tree state for next tests if needed
						if !dryRun {
							treeSavedStates[tt][gentype] = at
						}
					})
				}
			}
		}
	}
}
