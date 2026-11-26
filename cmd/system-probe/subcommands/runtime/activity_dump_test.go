// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package runtime

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestListActivityDumpsCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"runtime", "activity-dump", "list"},
		listActivityDumps,
		func() {})
}

func TestStopActivityDumpCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"runtime", "activity-dump", "stop"},
		stopActivityDump,
		func() {})
}

func TestGenerateEncodingFromActivityDumpCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"runtime", "activity-dump", "generate", "encoding", "--input", "file"},
		generateEncodingFromActivityDump,
		func() {})
}

func TestDiffActivityDumpCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"runtime", "activity-dump", "diff"},
		diffActivityDump,
		func() {})
}

func TestComputeActivityDumpDiffUnattributedActivity(t *testing.T) {
	newUnattributedActivity := func(domains ...string) *activity_tree.ProcessNode {
		ua := &activity_tree.ProcessNode{DNSNames: make(map[string]*activity_tree.DNSNode)}
		for _, domain := range domains {
			ua.DNSNames[domain] = &activity_tree.DNSNode{}
		}
		return ua
	}
	newDump := func(ua *activity_tree.ProcessNode) *profile.Profile {
		p := profile.New()
		p.ActivityTree = &activity_tree.ActivityTree{UnattributedActivity: ua}
		return p
	}
	stateOf := func(states map[string]bool, node *activity_tree.ProcessNode, dnsNode *activity_tree.DNSNode) (bool, bool) {
		id := utils.NewGraphID(utils.NewNodeIDFromPtr(node))
		if dnsNode != nil {
			id = id.Derive(utils.NewNodeIDFromPtr(dnsNode))
		}
		state, found := states[id.String()]
		return state, found
	}

	t.Run("added", func(t *testing.T) {
		target := newUnattributedActivity("added.com")
		states := make(map[string]bool)

		diff := computeActivityDumpDiff(newDump(nil), newDump(target), states)

		assert.Same(t, target, diff.ActivityTree.UnattributedActivity)
		state, found := stateOf(states, target, nil)
		assert.True(t, found)
		assert.True(t, state)
	})

	t.Run("removed", func(t *testing.T) {
		origin := newUnattributedActivity("removed.com")
		states := make(map[string]bool)

		diff := computeActivityDumpDiff(newDump(origin), newDump(nil), states)

		assert.Same(t, origin, diff.ActivityTree.UnattributedActivity)
		state, found := stateOf(states, origin, nil)
		assert.True(t, found)
		assert.False(t, state)
	})

	t.Run("in both dumps", func(t *testing.T) {
		origin := newUnattributedActivity("removed.com", "kept.com")
		target := newUnattributedActivity("kept.com", "added.com")
		states := make(map[string]bool)

		diff := computeActivityDumpDiff(newDump(origin), newDump(target), states)

		ua := diff.ActivityTree.UnattributedActivity
		require.NotNil(t, ua)
		require.Len(t, ua.DNSNames, 3)
		state, found := stateOf(states, ua, ua.DNSNames["added.com"])
		assert.True(t, found)
		assert.True(t, state)
		state, found = stateOf(states, ua, ua.DNSNames["removed.com"])
		assert.True(t, found)
		assert.False(t, state)
		_, found = stateOf(states, ua, ua.DNSNames["kept.com"])
		assert.False(t, found)
		_, found = stateOf(states, ua, nil)
		assert.False(t, found)
	})
}

func TestComputeActivityDumpDiffMatchedProcess(t *testing.T) {
	newProcess := func(path string, children ...*activity_tree.ProcessNode) *activity_tree.ProcessNode {
		pn := &activity_tree.ProcessNode{Children: children, DNSNames: make(map[string]*activity_tree.DNSNode)}
		pn.Process.FileEvent.PathnameStr = path
		return pn
	}
	removedChild := newProcess("/bin/removed")
	addedChild := newProcess("/bin/added")
	originRoot := newProcess("/bin/root", removedChild)
	originRoot.DNSNames["removed.com"] = &activity_tree.DNSNode{}
	targetRoot := newProcess("/bin/root", addedChild)
	targetRoot.DNSNames["added.com"] = &activity_tree.DNSNode{}
	origin := profile.New()
	origin.ActivityTree = &activity_tree.ActivityTree{ProcessNodes: []*activity_tree.ProcessNode{originRoot}}
	target := profile.New()
	target.ActivityTree = &activity_tree.ActivityTree{ProcessNodes: []*activity_tree.ProcessNode{targetRoot}}
	states := make(map[string]bool)

	diff := computeActivityDumpDiff(origin, target, states)

	require.Len(t, diff.ActivityTree.ProcessNodes, 1)
	root := diff.ActivityTree.ProcessNodes[0]
	assert.Len(t, root.Children, 2)
	assert.Equal(t, addedADNode, addedChild.GenerationType)
	assert.Equal(t, removedADNode, removedChild.GenerationType)
	rootID := utils.NewGraphID(utils.NewNodeIDFromPtr(root))
	for _, node := range []struct {
		name  string
		id    utils.GraphID
		added bool
	}{
		{"added child", utils.NewGraphID(utils.NewNodeIDFromPtr(addedChild)), true},
		{"removed child", utils.NewGraphID(utils.NewNodeIDFromPtr(removedChild)), false},
		{"added domain", rootID.Derive(utils.NewNodeIDFromPtr(root.DNSNames["added.com"])), true},
		{"removed domain", rootID.Derive(utils.NewNodeIDFromPtr(root.DNSNames["removed.com"])), false},
	} {
		state, found := states[node.id.String()]
		assert.True(t, found, node.name)
		assert.Equal(t, node.added, state, node.name)
	}
}
