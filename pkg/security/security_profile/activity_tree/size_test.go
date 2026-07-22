// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// newSizeTestTree builds an ActivityTree via NewActivityTree (so the cookie cache and
// SyscallsMask are wired up correctly — EvictImageTag panics otherwise) and the test
// validator used by the rest of the suite.
func newSizeTestTree() *ActivityTree {
	return NewActivityTree(activityTreeInsertTestValidator{}, nil, "security_profile")
}

// newSizeTestProcessNode returns a ProcessNode populated enough that size() exercises every
// branch in processStringsBytes — exec path, interpreter path, argv/envs, container, creds.
func newSizeTestProcessNode(name string) *ProcessNode {
	return &ProcessNode{
		NodeBase:       NewNodeBase(),
		Files:          map[string]*FileNode{},
		DNSNames:       map[string]*DNSNode{},
		IMDSEvents:     map[model.IMDSEvent]*IMDSNode{},
		NetworkDevices: map[model.NetworkDeviceContext]*NetworkDeviceNode{},
		Process: model.Process{
			FileEvent: model.FileEvent{
				PathnameStr: "/usr/bin/" + name,
				BasenameStr: name,
				Filesystem:  "ext4",
				PkgName:     "coreutils",
				PkgVersion:  "9.4",
				Hashes:      []string{"sha256:deadbeef"},
			},
			LinuxBinprm: model.LinuxBinprm{
				FileEvent: model.FileEvent{
					PathnameStr: "/usr/bin/python3",
					BasenameStr: "python3",
					Filesystem:  "ext4",
					PkgName:     "python3",
					PkgVersion:  "3.12",
					Hashes:      []string{"sha256:cafef00d"},
				},
			},
			Argv0:   name,
			Comm:    name,
			TTYName: "pts/0",
			Argv:    []string{"--flag", "value"},
			Envs:    []string{"PATH=/usr/bin"},
			Envp:    []string{"HOME=/root"},
			ContainerContext: model.ContainerContext{
				ContainerID: containerutils.ContainerID("container-" + name),
				Tags:        []string{"env:test", "service:" + name},
			},
			CGroup: model.CGroupContext{CGroupID: containerutils.CGroupID("cgroup-" + name)},
			Credentials: model.Credentials{
				User: "root", Group: "root",
				EUser: "root", EGroup: "root",
				FSUser: "root", FSGroup: "root",
			},
		},
	}
}

// newFileOpenEvent returns the smallest event payload that drives InsertFileEvent.
func newFileOpenEvent(path string) *model.Event {
	return &model.Event{
		BaseEvent: model.BaseEvent{FieldHandlers: &model.FakeFieldHandlers{}},
		Open: model.OpenEvent{
			File: model.FileEvent{IsPathnameStrResolved: true, PathnameStr: path},
		},
	}
}

func newDNSEvent(name string, qtype uint16) *model.Event {
	return &model.Event{
		BaseEvent: model.BaseEvent{FieldHandlers: &model.FakeFieldHandlers{}},
		DNS: model.DNSEvent{
			Question: model.DNSQuestion{Name: name, Type: qtype},
		},
	}
}

func newBindEvent(port uint16) *model.Event {
	return &model.Event{
		BaseEvent: model.BaseEvent{FieldHandlers: &model.FakeFieldHandlers{}},
		Bind: model.BindEvent{
			Addr:       model.IPPortContext{Port: port},
			AddrFamily: unix.AF_INET,
		},
	}
}

// populateRichTree adds a varied mix of nodes to the tree using the public Insert*Event
// methods so SizeBytes is updated incrementally. Returns the root ProcessNode for callers
// that want to mutate it further.
func populateRichTree(t *testing.T, tree *ActivityTree, tagID uint64) *ProcessNode {
	t.Helper()
	root := newSizeTestProcessNode("root")
	root.AppendImageTagID(tagID, time.Now())
	tree.ProcessNodes = []*ProcessNode{root}

	for _, path := range []string{"/etc/passwd", "/var/log/syslog", "/tmp/a/b/c/d"} {
		evt := newFileOpenEvent(path)
		root.InsertFileEvent(&evt.Open.File, evt, tagID, Runtime, tree.Stats, false, nil, nil)
	}

	for _, name := range []string{"datadoghq.com", "example.org"} {
		evt := newDNSEvent(name, 1)
		root.InsertDNSEvent(evt, tagID, Runtime, tree.Stats, tree.DNSNames, false, 0)
	}

	for _, port := range []uint16{80, 443, 8080} {
		evt := newBindEvent(port)
		root.InsertBindEvent(evt, tagID, Runtime, tree.Stats, false)
	}

	child := newSizeTestProcessNode("child")
	child.AppendImageTagID(tagID, time.Now())
	child.Parent = root
	root.Children = append(root.Children, child)
	tree.Stats.ProcessNodes++
	tree.Stats.SizeBytes += child.size()
	return root
}

// TestSizeBytes_EmptyTree confirms an empty tree reports zero on both the legacy V1
// estimate (ApproximateSize) and the V2 accurate estimate (HeapSize) so neither metric
// lies when no profile data exists yet.
func TestSizeBytes_EmptyTree(t *testing.T) {
	tree := newSizeTestTree()
	assert.Equal(t, int64(0), tree.Stats.SizeBytes)
	assert.Equal(t, int64(0), tree.Stats.ApproximateSize())
	assert.Equal(t, int64(0), tree.Stats.HeapSize())
}

// TestApproximateSize_LegacyShallowSemantics locks in V1's legacy behavior: ApproximateSize
// is *only* node counts × struct header sizes, regardless of SizeBytes. V1 thresholds
// (activity_dump.max_dump_size, anomaly_detection.unstable_profile_size_threshold) are
// tuned against this number — any drift here shifts production V1 behavior silently.
func TestApproximateSize_LegacyShallowSemantics(t *testing.T) {
	tests := []struct {
		name  string
		stats Stats
	}{
		{"process_only", Stats{ProcessNodes: 1}},
		{"file_only", Stats{FileNodes: 10}},
		{"dns_only", Stats{DNSNodes: 5}},
		{"socket_only", Stats{SocketNodes: 3}},
		{"imds_only", Stats{IMDSNodes: 2}},
		{"syscall_only", Stats{SyscallNodes: 4}},
		{"flow_only", Stats{FlowNodes: 7}},
		{"capability_only", Stats{CapabilityNodes: 6}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Greater(t, tt.stats.ApproximateSize(), int64(0),
				"every counted node type must contribute to ApproximateSize for %s", tt.name)
		})
	}
}

// TestApproximateSize_IgnoresSizeBytes nails down V1's invariant: ApproximateSize never
// consults SizeBytes, even when it's set. If a future refactor wires SizeBytes into
// ApproximateSize, V1 max-size and unstable-threshold behavior would change without anyone
// touching the V1 config knobs.
func TestApproximateSize_IgnoresSizeBytes(t *testing.T) {
	stats := Stats{ProcessNodes: 1, SizeBytes: 999999}
	expected := int64(unsafe.Sizeof(ProcessNode{}))
	assert.Equal(t, expected, stats.ApproximateSize(),
		"ApproximateSize must stay on the legacy shallow path (count × unsafe.Sizeof)")
}

// TestHeapSize_PrefersIncremental verifies V2's accurate path: once SizeBytes is populated,
// HeapSize returns it directly. Falls back to ApproximateSize only when SizeBytes is zero
// (test fixtures, proto rehydration before recompute fires).
func TestHeapSize_PrefersIncremental(t *testing.T) {
	stats := Stats{
		ProcessNodes: 1000, // would dominate the fallback
		SizeBytes:    42,
	}
	assert.Equal(t, int64(42), stats.HeapSize())
}

// TestHeapSize_FallsBackToApproximateSize verifies V2's fallback: when SizeBytes hasn't
// been populated yet, HeapSize must still produce a non-zero number so the V2 max-size
// check and the profile_size metric stay useful until recomputeSizeBytes runs.
func TestHeapSize_FallsBackToApproximateSize(t *testing.T) {
	stats := Stats{ProcessNodes: 1}
	require.Equal(t, int64(0), stats.SizeBytes)
	assert.Equal(t, stats.ApproximateSize(), stats.HeapSize(),
		"HeapSize must fall back to ApproximateSize when SizeBytes is unset")
}

// TestSizeBytes_IncrementalUndercountsByBoundedDrift documents the production design: the
// incremental tracker undercounts slightly because slice/map backing-array growth on parent
// containers isn't re-measured on each insert. recomputeSizeBytes is the source of truth and
// is intentionally heavier. We assert (a) incremental <= recompute (never overshoots) and
// (b) drift stays small relative to the total — if either condition breaks, an insert path
// either started double-charging or stopped charging entirely.
func TestSizeBytes_IncrementalUndercountsByBoundedDrift(t *testing.T) {
	tree := newSizeTestTree()
	tagID := tree.GetOrInsertImageTag("v1")
	populateRichTree(t, tree, tagID)

	incremental := tree.Stats.SizeBytes
	tree.recomputeSizeBytes()
	canonical := tree.Stats.SizeBytes

	require.Greater(t, canonical, int64(0), "non-empty tree should report non-zero size")
	assert.LessOrEqual(t, incremental, canonical,
		"incremental tracker (%d) should never exceed recompute (%d) — that would mean an insert path double-charged",
		incremental, canonical)
	// Drift comes from parent-container growth (Children/Sockets/Bind slice cap doubling,
	// Files/DNSNames/IMDSEvents map bucket overhead). It must stay a small fraction of the
	// total or the metric stops being useful before recompute corrects it.
	assert.Less(t, canonical-incremental, canonical/2,
		"drift (%d) shouldn't exceed half the canonical size (%d)", canonical-incremental, canonical)
}

// TestSizeBytes_EvictImageTag_RecomputesToZero asserts the most important invariant for
// the metric: once every tag is evicted and the tree is empty, recompute (the source of
// truth) reports zero. The incremental tracker may carry a small residual from backing-array
// growth not paid for at insert time — we let recompute reset it, which is how production
// works (recompute runs on every ComputeActivityTreeStats).
func TestSizeBytes_EvictImageTag_RecomputesToZero(t *testing.T) {
	tree := newSizeTestTree()
	tagID := tree.GetOrInsertImageTag("v1")
	populateRichTree(t, tree, tagID)
	require.Greater(t, tree.Stats.SizeBytes, int64(0))

	tree.EvictImageTag("v1")

	assert.Empty(t, tree.ProcessNodes, "every process node should have been evicted")
	tree.recomputeSizeBytes()
	assert.Equal(t, int64(0), tree.Stats.SizeBytes,
		"recompute on an empty tree must yield zero (got %d) — anything else means recompute is leaking",
		tree.Stats.SizeBytes)
}

// TestSizeBytes_EvictUnusedNodes_RecomputesToZero is the same invariant via the time-based
// eviction path. Push the cutoff far into the future so every node is stale, then verify
// recompute lands on zero.
func TestSizeBytes_EvictUnusedNodes_RecomputesToZero(t *testing.T) {
	tree := newSizeTestTree()
	tagID := tree.GetOrInsertImageTag("v1")
	populateRichTree(t, tree, tagID)
	require.Greater(t, tree.Stats.SizeBytes, int64(0))

	tree.EvictUnusedNodes(time.Now().Add(24*time.Hour), nil, "test-image", "v1")

	assert.Empty(t, tree.ProcessNodes, "every process node should have been evicted")
	tree.recomputeSizeBytes()
	assert.Equal(t, int64(0), tree.Stats.SizeBytes,
		"recompute on an empty tree must yield zero (got %d)", tree.Stats.SizeBytes)
}

// TestSizeBytes_DNSAppendChargesIncrementally locks in the invariant that adding a second
// query type to an existing DNSNode (which appends to dnsNode.Requests rather than
// allocating a new node) updates Stats.SizeBytes. The append path uses a before/after
// snapshot of dnsNode.size() — if a refactor drops that snapshot, recompute would diverge
// from incremental and eviction would over-subtract, causing the metric to drift negative.
// See process_node.go InsertDNSEvent for the snapshot.
func TestSizeBytes_DNSAppendChargesIncrementally(t *testing.T) {
	tree := newSizeTestTree()
	tagID := tree.GetOrInsertImageTag("v1")
	root := newSizeTestProcessNode("root")
	root.AppendImageTagID(tagID, time.Now())
	tree.ProcessNodes = []*ProcessNode{root}

	// First request: creates the DNSNode.
	first := newDNSEvent("api.datadoghq.com", 1) // A record
	root.InsertDNSEvent(first, tagID, Runtime, tree.Stats, tree.DNSNames, false, 0)
	sizeAfterFirst := tree.Stats.SizeBytes
	require.Greater(t, sizeAfterFirst, int64(0))

	// Second request for the same name but a different type: appends to dnsNode.Requests.
	// The new Question.Name length plus any slice-cap growth must show up in SizeBytes.
	second := newDNSEvent("api.datadoghq.com", 28) // AAAA record
	root.InsertDNSEvent(second, tagID, Runtime, tree.Stats, tree.DNSNames, false, 0)
	sizeAfterSecond := tree.Stats.SizeBytes

	assert.Greater(t, sizeAfterSecond, sizeAfterFirst,
		"appending a second DNS request (different type, same name) must grow Stats.SizeBytes — "+
			"otherwise recompute will diverge and eviction will over-subtract")

	// Recompute is the ground truth; incremental must still track within bounded drift.
	tree.recomputeSizeBytes()
	assert.LessOrEqual(t, sizeAfterSecond, tree.Stats.SizeBytes,
		"incremental (%d) should not exceed recompute (%d) after the append",
		sizeAfterSecond, tree.Stats.SizeBytes)
}

// TestSizeBytes_RecomputeIsIdempotent: calling recomputeSizeBytes twice on the same tree
// must produce the same number. Guards against accidental += instead of = in recompute.
func TestSizeBytes_RecomputeIsIdempotent(t *testing.T) {
	tree := newSizeTestTree()
	tagID := tree.GetOrInsertImageTag("v1")
	populateRichTree(t, tree, tagID)

	tree.recomputeSizeBytes()
	first := tree.Stats.SizeBytes
	tree.recomputeSizeBytes()
	second := tree.Stats.SizeBytes

	assert.Equal(t, first, second, "recomputeSizeBytes must be idempotent")
}
