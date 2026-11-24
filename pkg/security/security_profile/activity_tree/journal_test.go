// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package activitytree

import (
	"fmt"
	"net"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func newJournalTestProcessEntry(pid uint32, pathStr, baseStr, argv0 string, argv, envs []string, ancestor *model.ProcessCacheEntry) *model.ProcessCacheEntry {
	entry := model.NewPlaceholderProcessCacheEntry(pid, pid, false)
	entry.ContainerContext.ContainerID = "123"
	entry.FileEvent.PathnameStr = pathStr
	entry.FileEvent.BasenameStr = baseStr
	entry.FileEvent.Inode = uint64(pid) + 100
	entry.FileEvent.MountID = 1
	entry.FileEvent.Filesystem = "overlay"
	entry.FileEvent.PkgName = "test-pkg"
	entry.FileEvent.PkgVersion = "1.2.3-4"
	entry.FileEvent.Hashes = []string{
		"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"sha1:0123456789abcdef0123456789abcdef01234567",
	}
	entry.Argv0 = argv0
	entry.ArgsEntry = &model.ArgsEntry{Values: argv}
	entry.EnvsEntry = &model.EnvsEntry{Values: envs}
	entry.Envs = envs
	entry.Credentials = model.Credentials{
		UID: 1000, GID: 1000,
		User: "appuser", Group: "appgroup",
		EUser: "appuser", EGroup: "appgroup",
		FSUser: "appuser", FSGroup: "appgroup",
	}
	entry.Ancestor = ancestor
	return entry
}

func newJournalTestFileOpenEvent(entry *model.ProcessCacheEntry, pathStr string, flags uint32, mode uint16, ts time.Time) *model.Event {
	ev := model.NewFakeEvent()
	ev.Type = uint32(model.FileOpenEventType)
	ev.Timestamp = ts
	ev.TimestampRaw = uint64(ts.UnixNano())
	ev.ProcessCacheEntry = entry
	ev.ProcessContext = &entry.ProcessContext
	ev.Open.File.PathnameStr = pathStr
	ev.Open.File.BasenameStr = pathStr[stringsLastSlash(pathStr)+1:]
	ev.Open.File.Inode = 999
	ev.Open.File.MountID = 1
	ev.Open.File.Mode = mode
	ev.Open.File.FileFields.Mode = mode
	ev.Open.File.Filesystem = "ext4"
	ev.Open.File.User = "appuser"
	ev.Open.File.Group = "appgroup"
	ev.Open.File.PkgName = "libssl3"
	ev.Open.File.PkgVersion = "3.0.11-1~deb12u2"
	ev.Open.File.Hashes = []string{
		"sha256:fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210",
	}
	ev.Open.Flags = flags
	ev.Open.Mode = uint32(mode)
	return ev
}

func stringsLastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}

func journalIPv4Net(ip string) net.IPNet {
	parsed := net.ParseIP(ip).To4()
	return net.IPNet{IP: parsed, Mask: net.CIDRMask(32, 32)}
}

func TestActivityTreeJournal_ParityWithInMemoryAndPatterns(t *testing.T) {
	reducer := NewPathsReducer()

	memTree := NewActivityTree(activityTreeInsertTestValidator{}, reducer, "security_profile")
	memTree.DifferentiateArgs()
	memTree.Stats.SetPathPatternConfig(DefaultPathPatternConfig())

	jTree := NewActivityTree(activityTreeInsertTestValidator{}, reducer, "security_profile")
	jTree.DifferentiateArgs()
	jTree.Stats.SetPathPatternConfig(DefaultPathPatternConfig())
	require.NoError(t, jTree.EnableJournal(t.TempDir(), "nginx:v1", 0))
	t.Cleanup(func() { _ = jTree.CloseJournal() })

	shim := newJournalTestProcessEntry(1, "/usr/bin/containerd-shim-runc-v2", "containerd-shim-runc-v2", "containerd-shim-runc-v2", nil, nil, nil)
	worker := newJournalTestProcessEntry(
		100,
		"/usr/bin/nginx",
		"nginx",
		"nginx",
		[]string{"-c", "/etc/nginx/nginx.conf", "-g", "daemon off;"},
		[]string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin", "NGINX_VERSION=1.25.3"},
		shim,
	)

	t1 := time.Unix(1700000000, 0)
	t2 := t1.Add(10 * time.Second)

	execEv := model.NewFakeEvent()
	execEv.Type = uint32(model.ExecEventType)
	execEv.Timestamp = t1
	execEv.TimestampRaw = uint64(t1.UnixNano())
	execEv.ProcessCacheEntry = worker
	execEv.ProcessContext = &worker.ProcessContext
	execEv.Exec.Process = &worker.Process
	execEv.Rules = []*model.MatchedRule{{RuleID: "exec_rule_1", RuleVersion: "1.0"}}

	insMem, _, _, err := memTree.Insert(execEv, true, "v1", Runtime, nil)
	require.NoError(t, err)
	insJ, _, _, err := jTree.Insert(execEv, true, "v1", Runtime, nil)
	require.NoError(t, err)
	assert.Equal(t, insMem, insJ)

	require.Len(t, jTree.ProcessNodes, 1)
	assert.Empty(t, jTree.ProcessNodes[0].Process.Argv)
	assert.Empty(t, jTree.ProcessNodes[0].Process.Envs)
	assert.Empty(t, jTree.ProcessNodes[0].MatchedRules)

	contains, err := jTree.Contains(execEv, true, "v1", Runtime, nil)
	require.NoError(t, err)
	assert.True(t, contains)

	for i := 1; i <= 4; i++ {
		fEv := newJournalTestFileOpenEvent(worker, fmt.Sprintf("/var/job/%d/out.log", i), syscall.O_RDONLY, syscall.S_IFREG|0644, t1)
		ins1, _, _, err := memTree.Insert(fEv, true, "v1", Runtime, nil)
		require.NoError(t, err)
		ins2, _, _, err := jTree.Insert(fEv, true, "v1", Runtime, nil)
		require.NoError(t, err)
		assert.Equal(t, ins1, ins2)
	}

	fEvEnrich := newJournalTestFileOpenEvent(worker, "/var/job/1/out.log", syscall.O_WRONLY|syscall.O_APPEND, syscall.S_IFREG|0644, t2)
	fEvEnrich.Rules = []*model.MatchedRule{{RuleID: "fim_write_rule", RuleVersion: "1.0"}}
	ins1, _, _, err := memTree.Insert(fEvEnrich, true, "v2", Runtime, nil)
	require.NoError(t, err)
	ins2, _, _, err := jTree.Insert(fEvEnrich, true, "v2", Runtime, nil)
	require.NoError(t, err)
	assert.False(t, ins1)
	assert.False(t, ins2)
	assert.Empty(t, jTree.ProcessNodes[0].Files["var"].Children["job"].Children["1"].Children["out.log"].MatchedRules, "enriched file rules must be spilled and stripped from RAM")

	var headNodeBase, foldedNodeBase *NodeBase
	for i := 1; i <= 16; i++ {
		fEv := newJournalTestFileOpenEvent(worker, fmt.Sprintf("/var/cache/item-%d.dat", i), syscall.O_RDONLY, syscall.S_IFREG|0644, t1)
		_, _, _, err := memTree.Insert(fEv, true, "v1", Runtime, nil)
		require.NoError(t, err)
		_, _, nb, err := jTree.Insert(fEv, true, "v1", Runtime, nil)
		require.NoError(t, err)
		if i == 1 {
			headNodeBase = nb
		} else if i == 2 {
			foldedNodeBase = nb
		}
	}
	moved := jTree.Stats.TakeMovedNodes()
	require.NotEmpty(t, moved)
	assert.Same(t, headNodeBase, moved[foldedNodeBase], "merged pattern node must record moved NodeBase for sample cookie retargeting")

	fEv999 := newJournalTestFileOpenEvent(worker, "/var/cache/item-999.dat", syscall.O_RDONLY, syscall.S_IFREG|0644, t2)
	contains, err = jTree.Contains(fEv999, true, "v1", Runtime, nil)
	require.NoError(t, err)
	assert.True(t, contains)

	dnsEv1 := model.NewFakeEvent()
	dnsEv1.Type = uint32(model.DNSEventType)
	dnsEv1.Timestamp = t1
	dnsEv1.ProcessCacheEntry = worker
	dnsEv1.ProcessContext = &worker.ProcessContext
	dnsEv1.DNS.Question = model.DNSQuestion{Name: "api.datadoghq.com", Type: 1, Class: 1, Size: 32, Count: 1}
	dnsEv1.DNS.Response = &model.DNSResponse{
		IPs:    []net.IPNet{journalIPv4Net("10.0.0.1")},
		CNames: []string{"edge.datadoghq.com"},
	}
	_, _, _, err = memTree.Insert(dnsEv1, true, "v1", Runtime, nil)
	require.NoError(t, err)
	_, _, _, err = jTree.Insert(dnsEv1, true, "v1", Runtime, nil)
	require.NoError(t, err)

	dnsEv2 := *dnsEv1
	dnsEv2.Timestamp = t2
	dnsEv2.DNS.Response = &model.DNSResponse{
		IPs:    []net.IPNet{journalIPv4Net("10.0.0.2")},
		CNames: []string{"edge.datadoghq.com"},
	}
	_, _, _, err = memTree.Insert(&dnsEv2, true, "v1", Runtime, nil)
	require.NoError(t, err)
	_, _, _, err = jTree.Insert(&dnsEv2, true, "v1", Runtime, nil)
	require.NoError(t, err)

	assert.Nil(t, jTree.ProcessNodes[0].DNSNames["api.datadoghq.com"].Requests[0].Response)

	memTree.FinalizePatterns()
	jTree.FinalizePatterns()

	memProto := ToProto(memTree)
	jProto := ToProto(jTree)
	require.Len(t, memProto, 1)
	require.Len(t, jProto, 1)

	assert.Empty(t, jTree.ProcessNodes[0].Process.Argv)
	assert.Nil(t, jTree.ProcessNodes[0].DNSNames["api.datadoghq.com"].Requests[0].Response)
	assert.Less(t, jTree.Stats.HeapSize(), memTree.Stats.HeapSize())

	decoded := NewActivityTree(activityTreeInsertTestValidator{}, reducer, "security_profile")
	ProtoDecodeActivityTree(decoded, jProto)
	require.Len(t, decoded.ProcessNodes, 1)
	dp := decoded.ProcessNodes[0]
	mp := memTree.ProcessNodes[0]

	assert.Equal(t, mp.Process.FileEvent.PathnameStr, dp.Process.FileEvent.PathnameStr)
	assert.Equal(t, mp.Process.Argv, dp.Process.Argv)
	assert.Equal(t, mp.Process.Envs, dp.Process.Envs)
	assert.Equal(t, mp.Process.Credentials, dp.Process.Credentials)
	assert.Equal(t, mp.Process.FileEvent.Hashes, dp.Process.FileEvent.Hashes)
	require.Len(t, dp.MatchedRules, 1)
	assert.Equal(t, "exec_rule_1", dp.MatchedRules[0].RuleID)

	numNode := dp.Files["var"].Children["job"].Children["<num>"]
	require.NotNil(t, numNode)
	assert.True(t, numNode.IsPattern)
	outLog := numNode.Children["out.log"]
	require.NotNil(t, outLog)
	require.NotNil(t, outLog.File)
	assert.Equal(t, "/var/job/<num>/out.log", outLog.File.PathnameStr)
	assert.Equal(t, "libssl3", outLog.File.PkgName)
	require.NotNil(t, outLog.Open)
	assert.Equal(t, uint32(syscall.O_RDONLY|syscall.O_WRONLY|syscall.O_APPEND), outLog.Open.Flags)
	require.Len(t, outLog.MatchedRules, 1)
	assert.Equal(t, "fim_write_rule", outLog.MatchedRules[0].RuleID)

	dnsNode := dp.DNSNames["api.datadoghq.com"]
	require.NotNil(t, dnsNode)
	require.Len(t, dnsNode.Requests, 1)
	require.NotNil(t, dnsNode.Requests[0].Response)
	assert.ElementsMatch(t, []net.IPNet{journalIPv4Net("10.0.0.1"), journalIPv4Net("10.0.0.2")}, dnsNode.Requests[0].Response.IPs)
	assert.Equal(t, []string{"edge.datadoghq.com"}, dnsNode.Requests[0].Response.CNames)
}

func TestActivityTreeJournal_EvictionAndABARecreation(t *testing.T) {
	jTree := NewActivityTree(activityTreeInsertTestValidator{}, NewPathsReducer(), "security_profile")
	require.NoError(t, jTree.EnableJournal(t.TempDir(), "aba:v1", 0))
	t.Cleanup(func() { _ = jTree.CloseJournal() })

	shim := newJournalTestProcessEntry(1, "/usr/bin/containerd-shim-runc-v2", "containerd-shim-runc-v2", "containerd-shim-runc-v2", nil, nil, nil)
	worker := newJournalTestProcessEntry(100, "/usr/bin/app", "app", "app", []string{"--serve"}, nil, shim)

	t1 := time.Unix(1700000000, 0)
	t2 := t1.Add(time.Minute)

	evV1 := newJournalTestFileOpenEvent(worker, "/tmp/ephemeral.txt", syscall.O_CREAT|syscall.O_WRONLY, syscall.S_IFREG|0600, t1)
	evV1.Rules = []*model.MatchedRule{{RuleID: "v1_stale_rule", RuleVersion: "1.0"}}
	_, _, _, err := jTree.Insert(evV1, true, "v1", Runtime, nil)
	require.NoError(t, err)

	evV2 := newJournalTestFileOpenEvent(worker, "/tmp/keep.txt", syscall.O_RDONLY, syscall.S_IFREG|0644, t2)
	_, _, _, err = jTree.Insert(evV2, true, "v2", Runtime, nil)
	require.NoError(t, err)

	jTree.EvictImageTag("v1")

	protoAfterEvict := ToProto(jTree)
	decoded1 := NewActivityTree(activityTreeInsertTestValidator{}, nil, "security_profile")
	ProtoDecodeActivityTree(decoded1, protoAfterEvict)
	require.Len(t, decoded1.ProcessNodes, 1)
	assert.Nil(t, decoded1.ProcessNodes[0].Files["tmp"].Children["ephemeral.txt"], "evicted v1 file must not be rehydrated from journal")
	assert.NotNil(t, decoded1.ProcessNodes[0].Files["tmp"].Children["keep.txt"])

	evV2Recreate := newJournalTestFileOpenEvent(worker, "/tmp/ephemeral.txt", syscall.O_RDONLY, syscall.S_IFREG|0644, t2)
	_, _, _, err = jTree.Insert(evV2Recreate, true, "v2", Runtime, nil)
	require.NoError(t, err)

	protoAfterRecreate := ToProto(jTree)
	decoded2 := NewActivityTree(activityTreeInsertTestValidator{}, nil, "security_profile")
	ProtoDecodeActivityTree(decoded2, protoAfterRecreate)
	ephemeral := decoded2.ProcessNodes[0].Files["tmp"].Children["ephemeral.txt"]
	require.NotNil(t, ephemeral)
	require.NotNil(t, ephemeral.Open)
	assert.Equal(t, uint32(syscall.O_RDONLY), ephemeral.Open.Flags, "stale pre-eviction flags must not leak into recreated node")
	assert.Empty(t, ephemeral.MatchedRules, "stale pre-eviction rules must not leak into recreated node")

	jTree.Stats.SetPathPatternConfig(DefaultPathPatternConfig())
	for i := 1; i <= 3; i++ {
		fEv := newJournalTestFileOpenEvent(worker, fmt.Sprintf("/tmp/fold-%d.log", i), syscall.O_RDONLY, syscall.S_IFREG|0644, t2)
		_, _, _, err = jTree.Insert(fEv, true, "v2", Runtime, nil)
		require.NoError(t, err)
	}
	evStaleFold := newJournalTestFileOpenEvent(worker, "/tmp/fold-4.log", syscall.O_WRONLY, syscall.S_IFREG|0644, t1)
	evStaleFold.Rules = []*model.MatchedRule{{RuleID: "v1_stale_fold_rule", RuleVersion: "1.0"}}
	_, _, _, err = jTree.Insert(evStaleFold, true, "v1", Runtime, nil)
	require.NoError(t, err)
	jTree.EvictImageTag("v1")

	evFreshFold := newJournalTestFileOpenEvent(worker, "/tmp/fold-4.log", syscall.O_RDONLY, syscall.S_IFREG|0644, t2)
	_, _, _, err = jTree.Insert(evFreshFold, true, "v2", Runtime, nil)
	require.NoError(t, err)

	jTree.FinalizePatterns()
	protoAfterFold := ToProto(jTree)
	decoded3 := NewActivityTree(activityTreeInsertTestValidator{}, nil, "security_profile")
	ProtoDecodeActivityTree(decoded3, protoAfterFold)
	folded := decoded3.ProcessNodes[0].Files["tmp"].Children["fold-<num>.log"]
	require.NotNil(t, folded)
	assert.Empty(t, folded.MatchedRules, "evicted sibling rule must not be replayed onto folded pattern node")
}

func TestActivityTreeJournal_MemoryReductionAndSizeBytesInvariants(t *testing.T) {
	reducer := NewPathsReducer()
	memTree := NewActivityTree(activityTreeInsertTestValidator{}, reducer, "security_profile")
	jTree := NewActivityTree(activityTreeInsertTestValidator{}, reducer, "security_profile")
	require.NoError(t, jTree.EnableJournal(t.TempDir(), "mem-bench:v1", 0))
	t.Cleanup(func() { _ = jTree.CloseJournal() })

	shim := newJournalTestProcessEntry(1, "/usr/bin/containerd-shim-runc-v2", "containerd-shim-runc-v2", "containerd-shim-runc-v2", nil, nil, nil)
	ts := time.Unix(1700000000, 0)

	for p := 0; p < 25; p++ {
		proc := newJournalTestProcessEntry(
			uint32(100+p),
			fmt.Sprintf("/usr/local/bin/service-worker-%d", p),
			fmt.Sprintf("service-worker-%d", p),
			fmt.Sprintf("service-worker-%d", p),
			[]string{"--config", "/etc/service/config.yaml", "--workers", "8", "--log-level", "info"},
			[]string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin", "DD_ENV=prod", "DD_SERVICE=checkout"},
			shim,
		)
		for f := 0; f < 40; f++ {
			fEv := newJournalTestFileOpenEvent(proc, fmt.Sprintf("/var/lib/service/worker-%d/shard-%d/data-%d.db", p, f%5, f), syscall.O_RDWR, syscall.S_IFREG|0640, ts)
			_, _, _, err := memTree.Insert(fEv, true, "v1", Runtime, nil)
			require.NoError(t, err)
			_, _, _, err = jTree.Insert(fEv, true, "v1", Runtime, nil)
			require.NoError(t, err)
		}
		for d := 0; d < 10; d++ {
			dnsEv := model.NewFakeEvent()
			dnsEv.Type = uint32(model.DNSEventType)
			dnsEv.Timestamp = ts
			dnsEv.ProcessCacheEntry = proc
			dnsEv.ProcessContext = &proc.ProcessContext
			dnsEv.DNS.Question = model.DNSQuestion{Name: fmt.Sprintf("dep-%d.internal.example.com", d), Type: 1, Class: 1, Size: 40, Count: 1}
			dnsEv.DNS.Response = &model.DNSResponse{
				IPs: []net.IPNet{
					journalIPv4Net(fmt.Sprintf("10.%d.%d.1", p, d)),
					journalIPv4Net(fmt.Sprintf("10.%d.%d.2", p, d)),
					journalIPv4Net(fmt.Sprintf("10.%d.%d.3", p, d)),
					journalIPv4Net(fmt.Sprintf("10.%d.%d.4", p, d)),
				},
				CNames: []string{fmt.Sprintf("cname-%d.internal.example.com", d)},
			}
			_, _, _, err := memTree.Insert(dnsEv, true, "v1", Runtime, nil)
			require.NoError(t, err)
			_, _, _, err = jTree.Insert(dnsEv, true, "v1", Runtime, nil)
			require.NoError(t, err)
		}
	}

	memIncremental := memTree.Stats.SizeBytes
	memTree.recomputeSizeBytes()
	memCanonical := memTree.Stats.SizeBytes

	jIncremental := jTree.Stats.SizeBytes
	jTree.recomputeSizeBytes()
	jCanonical := jTree.Stats.SizeBytes

	t.Logf("Unjournaled ActivityTree HeapSize: %d bytes (%.2f KB, incremental=%d)", memCanonical, float64(memCanonical)/1024.0, memIncremental)
	t.Logf("Journaled ActivityTree HeapSize:   %d bytes (%.2f KB, incremental=%d, savings=%.1f%%)",
		jCanonical, float64(jCanonical)/1024.0, jIncremental, 100.0*(1.0-float64(jCanonical)/float64(memCanonical)))

	assert.LessOrEqual(t, jIncremental, jCanonical, "incremental SizeBytes on journaled tree must never exceed recompute")
	assert.Less(t, jCanonical, memCanonical*60/100, "journaled ActivityTree should use at least 40%% less RAM")

	jTree.EvictImageTag("v1")
	jTree.recomputeSizeBytes()
	assert.Equal(t, int64(0), jTree.Stats.SizeBytes)
}

func TestActivityTreeJournal_IMDSRulesAndRuleVersionIdentity(t *testing.T) {
	jTree := NewActivityTree(activityTreeInsertTestValidator{}, NewPathsReducer(), "security_profile")
	require.NoError(t, jTree.EnableJournal(t.TempDir(), "imds-rules:v1", 0))
	t.Cleanup(func() { _ = jTree.CloseJournal() })

	shim := newJournalTestProcessEntry(1, "/usr/bin/containerd-shim-runc-v2", "containerd-shim-runc-v2", "containerd-shim-runc-v2", nil, nil, nil)
	worker := newJournalTestProcessEntry(100, "/usr/bin/app", "app", "app", []string{"--serve"}, nil, shim)
	ts := time.Unix(1700000000, 0)

	imdsEv1 := model.NewFakeEvent()
	imdsEv1.Type = uint32(model.IMDSEventType)
	imdsEv1.Timestamp = ts
	imdsEv1.ProcessCacheEntry = worker
	imdsEv1.ProcessContext = &worker.ProcessContext
	imdsEv1.IMDS = model.IMDSEvent{
		Type:          "request",
		CloudProvider: "aws",
		URL:           "http://169.254.169.254/latest/meta-data/iam/security-credentials/role",
	}
	imdsEv1.Rules = []*model.MatchedRule{
		{RuleID: "imds_access", RuleVersion: "1.0", PolicyName: "default", PolicyVersion: "1"},
	}

	_, _, _, err := jTree.Insert(imdsEv1, true, "v1", Runtime, nil)
	require.NoError(t, err)

	imdsEv2 := *imdsEv1
	imdsEv2.Timestamp = ts.Add(time.Second)
	imdsEv2.Rules = []*model.MatchedRule{
		{RuleID: "imds_access", RuleVersion: "2.0", PolicyName: "default", PolicyVersion: "2"},
	}
	_, _, _, err = jTree.Insert(&imdsEv2, true, "v1", Runtime, nil)
	require.NoError(t, err)

	jTree.Stats.journal.dirty = true
	protoNodes := ToProto(jTree)
	require.Len(t, protoNodes, 1)

	decoded := NewActivityTree(activityTreeInsertTestValidator{}, nil, "security_profile")
	ProtoDecodeActivityTree(decoded, protoNodes)
	require.Len(t, decoded.ProcessNodes, 1)
	require.Len(t, decoded.ProcessNodes[0].IMDSEvents, 1)
	for _, in := range decoded.ProcessNodes[0].IMDSEvents {
		require.Len(t, in.MatchedRules, 2)
		assert.ElementsMatch(t, []string{"1.0", "2.0"}, []string{
			in.MatchedRules[0].RuleVersion,
			in.MatchedRules[1].RuleVersion,
		})
	}
}

func TestActivityTreeJournal_CapFallbackPreservesDNSEnrichment(t *testing.T) {
	jTree := NewActivityTree(activityTreeInsertTestValidator{}, NewPathsReducer(), "security_profile")
	require.NoError(t, jTree.EnableJournal(t.TempDir(), "cap-fallback:v1", 350))
	t.Cleanup(func() { _ = jTree.CloseJournal() })

	shim := newJournalTestProcessEntry(1, "/usr/bin/containerd-shim-runc-v2", "containerd-shim-runc-v2", "containerd-shim-runc-v2", nil, nil, nil)
	worker := newJournalTestProcessEntry(100, "/usr/bin/app", "app", "app", []string{"--serve"}, nil, shim)
	ts := time.Unix(1700000000, 0)

	dnsEv1 := model.NewFakeEvent()
	dnsEv1.Type = uint32(model.DNSEventType)
	dnsEv1.Timestamp = ts
	dnsEv1.ProcessCacheEntry = worker
	dnsEv1.ProcessContext = &worker.ProcessContext
	dnsEv1.DNS.Question = model.DNSQuestion{Name: "api.datadoghq.com", Type: 1, Class: 1, Size: 32, Count: 1}
	dnsEv1.DNS.Response = &model.DNSResponse{
		IPs: []net.IPNet{journalIPv4Net("10.0.0.1")},
	}
	_, _, _, err := jTree.Insert(dnsEv1, true, "v1", Runtime, nil)
	require.NoError(t, err)

	for i := 0; i < 10; i++ {
		fEv := newJournalTestFileOpenEvent(worker, fmt.Sprintf("/var/lib/large-payload-path-%d/file.txt", i), syscall.O_RDONLY, syscall.S_IFREG|0644, ts)
		_, _, _, err = jTree.Insert(fEv, true, "v1", Runtime, nil)
		require.NoError(t, err)
	}
	require.False(t, jTree.HasJournal())

	dnsEv2 := *dnsEv1
	dnsEv2.Timestamp = ts.Add(time.Second)
	dnsEv2.DNS.Response = &model.DNSResponse{
		IPs: []net.IPNet{journalIPv4Net("10.0.0.2")},
	}
	dnsEv2.Rules = []*model.MatchedRule{{RuleID: "dns_post_cap_rule", RuleVersion: "1.0"}}
	_, _, _, err = jTree.Insert(&dnsEv2, true, "v1", Runtime, nil)
	require.NoError(t, err)

	protoNodes := ToProto(jTree)
	decoded := NewActivityTree(activityTreeInsertTestValidator{}, nil, "security_profile")
	ProtoDecodeActivityTree(decoded, protoNodes)
	require.Len(t, decoded.ProcessNodes, 1)
	dn := decoded.ProcessNodes[0].DNSNames["api.datadoghq.com"]
	require.NotNil(t, dn)
	require.Len(t, dn.MatchedRules, 1)
	assert.Equal(t, "dns_post_cap_rule", dn.MatchedRules[0].RuleID)
	require.Len(t, dn.Requests, 1)
	require.NotNil(t, dn.Requests[0].Response)
	assert.ElementsMatch(t, []net.IPNet{journalIPv4Net("10.0.0.1"), journalIPv4Net("10.0.0.2")}, dn.Requests[0].Response.IPs)
}

func TestActivityTreeJournal_NonUTF8ArgsHashAndCheckpointRollback(t *testing.T) {
	dir := t.TempDir()
	jTree := NewActivityTree(activityTreeInsertTestValidator{}, NewPathsReducer(), "security_profile")
	jTree.DifferentiateArgs()
	require.NoError(t, jTree.EnableJournal(dir, "non-utf8:v1", 0))
	t.Cleanup(func() {
		_ = jTree.CloseJournal()
	})

	shim := newJournalTestProcessEntry(1, "/usr/bin/containerd-shim-runc-v2", "containerd-shim-runc-v2", "containerd-shim-runc-v2", nil, nil, nil)
	rawArgs := []string{"--raw=\xff\xfe\xfd", "--mode=prod"}
	worker := newJournalTestProcessEntry(100, "/usr/bin/app", "app", "app", rawArgs, []string{"ENV=1"}, shim)
	ts := time.Unix(1700000000, 0)

	fEv := newJournalTestFileOpenEvent(worker, "/etc/app.conf", syscall.O_RDONLY, syscall.S_IFREG|0644, ts)
	_, _, _, err := jTree.Insert(fEv, true, "v1", Runtime, nil)
	require.NoError(t, err)

	_ = ToProto(jTree)

	contains, err := jTree.Contains(fEv, true, "v1", Runtime, nil)
	require.NoError(t, err)
	assert.True(t, contains, "non-UTF-8 process argv must continue matching its existing ProcessNode after ToProto")

	jTree.Stats.journal.dirty = true
	require.NoError(t, os.Mkdir(jTree.Stats.journal.path+".tmp", 0750))

	protoNodes := ToProto(jTree)
	require.Len(t, protoNodes, 1)
	assert.NotEmpty(t, jTree.ProcessNodes[0].Process.Envs, "failed checkpoint must keep materialized payloads in RAM instead of stripping")
}

func TestActivityTreeJournal_SocketNetdevCloseAndCompaction(t *testing.T) {
	jTree := NewActivityTree(activityTreeInsertTestValidator{}, NewPathsReducer(), "security_profile")
	require.NoError(t, jTree.EnableJournal(t.TempDir(), "sock-netdev:v1", 2048))
	t.Cleanup(func() { _ = jTree.CloseJournal() })

	shim := newJournalTestProcessEntry(1, "/usr/bin/containerd-shim-runc-v2", "containerd-shim-runc-v2", "containerd-shim-runc-v2", nil, nil, nil)
	worker := newJournalTestProcessEntry(100, "/usr/bin/app", "app", "app", []string{"app", "--serve"}, []string{"ENV=1"}, shim)
	ts := time.Unix(1700000000, 0)

	bindEv := model.NewFakeEvent()
	bindEv.Type = uint32(model.BindEventType)
	bindEv.Timestamp = ts
	bindEv.ProcessCacheEntry = worker
	bindEv.ProcessContext = &worker.ProcessContext
	bindEv.Bind.AddrFamily = syscall.AF_INET
	bindEv.Bind.Addr.IPNet = journalIPv4Net("127.0.0.1")
	bindEv.Bind.Addr.Port = 8080
	bindEv.Bind.Protocol = syscall.IPPROTO_TCP
	bindEv.Rules = []*model.MatchedRule{{RuleID: "bind_rule", RuleVersion: "1.0"}}
	_, _, _, err := jTree.Insert(bindEv, true, "v1", Runtime, nil)
	require.NoError(t, err)

	flowEv1 := model.NewFakeEvent()
	flowEv1.Type = uint32(model.NetworkFlowMonitorEventType)
	flowEv1.Timestamp = ts
	flowEv1.ProcessCacheEntry = worker
	flowEv1.ProcessContext = &worker.ProcessContext
	flowEv1.NetworkFlowMonitor.Device = model.NetworkDeviceContext{NetNS: 42, IfIndex: 2, IfName: "eth0"}
	flowEv1.NetworkFlowMonitor.FlowsCount = 1
	flowEv1.NetworkFlowMonitor.Flows = []model.Flow{{
		L3Protocol: syscall.ETH_P_IP,
		L4Protocol: syscall.IPPROTO_TCP,
	}}
	flowEv1.Rules = []*model.MatchedRule{{RuleID: "stale_netdev_rule", RuleVersion: "1.0"}}
	_, _, _, err = jTree.Insert(flowEv1, true, "v1", Runtime, nil)
	require.NoError(t, err)

	fEvPermanent := newJournalTestFileOpenEvent(worker, "/etc/keep.conf", syscall.O_RDONLY, syscall.S_IFREG|0644, ts)
	_, _, _, err = jTree.Insert(fEvPermanent, true, "v2", Runtime, nil)
	require.NoError(t, err)

	jTree.EvictImageTag("v1")
	require.True(t, jTree.Stats.journal.dirty)

	flowEv2 := *flowEv1
	flowEv2.Timestamp = ts.Add(time.Second)
	flowEv2.Rules = nil
	_, _, _, err = jTree.Insert(&flowEv2, true, "v2", Runtime, nil)
	require.NoError(t, err)

	for i := 0; i < 6; i++ {
		fEv := newJournalTestFileOpenEvent(worker, fmt.Sprintf("/var/lib/compact-%d/data.bin", i), syscall.O_RDONLY, syscall.S_IFREG|0644, ts)
		_, _, _, err = jTree.Insert(fEv, true, "v2", Runtime, nil)
		require.NoError(t, err)
	}
	assert.True(t, jTree.HasJournal(), "dirty journal must compact before hitting size cap")

	protoNodes := ToProto(jTree)
	require.Len(t, protoNodes, 1)
	require.Len(t, protoNodes[0].NetworkDevices, 1)
	assert.Empty(t, protoNodes[0].NetworkDevices[0].MatchedRules, "evicted netdev rule must not attach to recreated netdev node")

	require.NoError(t, jTree.CloseJournal())
	assert.False(t, jTree.HasJournal())
	assert.Empty(t, jTree.ProcessNodes[0].Process.Argv, "CloseJournal must dispose of the journal without rehydrating stripped payloads")
}

func TestActivityTreeJournal_IncrementalFragmentationCompactionAndAsyncWriteFallback(t *testing.T) {
	jTree := NewActivityTree(activityTreeInsertTestValidator{}, NewPathsReducer(), "security_profile")
	require.NoError(t, jTree.EnableJournal(t.TempDir(), "frag-compact:v1", 8192))
	t.Cleanup(func() { _ = jTree.CloseJournal() })

	shim := newJournalTestProcessEntry(1, "/usr/bin/containerd-shim-runc-v2", "containerd-shim-runc-v2", "containerd-shim-runc-v2", nil, nil, nil)
	worker := newJournalTestProcessEntry(100, "/usr/bin/app", "app", "app", []string{"app", "--serve"}, []string{"ENV=1"}, shim)
	ts := time.Unix(1700000000, 0)

	fEv := newJournalTestFileOpenEvent(worker, "/var/lib/app/state.db", syscall.O_RDWR, syscall.S_IFREG|0640, ts)
	_, _, _, err := jTree.Insert(fEv, true, "v1", Runtime, nil)
	require.NoError(t, err)

	for i := 0; i < 30; i++ {
		ruleEv := newJournalTestFileOpenEvent(worker, "/var/lib/app/state.db", syscall.O_RDWR, syscall.S_IFREG|0640, ts.Add(time.Duration(i)*time.Second))
		ruleEv.Rules = []*model.MatchedRule{{
			RuleID:        fmt.Sprintf("incremental_rule_%d", i),
			RuleVersion:   "1.0",
			PolicyName:    "default",
			PolicyVersion: "1",
		}}
		_, _, _, err = jTree.Insert(ruleEv, true, "v1", Runtime, nil)
		require.NoError(t, err)
	}

	require.False(t, jTree.Stats.journal.dirty, "no evictions or pattern folds occurred")
	uncompactedSize := jTree.JournalSize()
	require.Greater(t, jTree.Stats.journal.fragmentedBytes, int64(2048))

	protoNodes := ToProto(jTree)
	require.Len(t, protoNodes, 1)
	compactedSize := jTree.JournalSize()
	assert.Less(t, compactedSize, uncompactedSize, "ToProto flush must compact fragmented incremental frames even when !dirty")
	assert.Equal(t, int64(0), jTree.Stats.journal.fragmentedBytes)

	decoded := NewActivityTree(activityTreeInsertTestValidator{}, nil, "security_profile")
	ProtoDecodeActivityTree(decoded, protoNodes)
	leaf := decoded.ProcessNodes[0].Files["var"].Children["lib"].Children["app"].Children["state.db"]
	require.NotNil(t, leaf)
	assert.Len(t, leaf.MatchedRules, 30)

	// Simulate an async write failure (e.g. disk full / read-only remount) while keeping existing diskBytes readable.
	roFile, err := os.OpenFile(jTree.Stats.journal.path, os.O_RDONLY, 0600)
	require.NoError(t, err)
	jTree.Stats.journal.ioMu.Lock()
	oldFile := jTree.Stats.journal.file
	jTree.Stats.journal.file = roFile
	jTree.Stats.journal.ioMu.Unlock()
	_ = oldFile.Close()

	ruleEvAfterClose := newJournalTestFileOpenEvent(worker, "/var/lib/app/state.db", syscall.O_RDWR, syscall.S_IFREG|0640, ts.Add(time.Minute))
	ruleEvAfterClose.Rules = []*model.MatchedRule{{
		RuleID:        "rule_in_fallback_buf",
		RuleVersion:   "1.0",
		PolicyName:    "default",
		PolicyVersion: "1",
	}}
	_, _, _, err = jTree.Insert(ruleEvAfterClose, true, "v1", Runtime, nil)
	require.NoError(t, err)

	protoAfterFail := ToProto(jTree)
	assert.Equal(t, int64(0), jTree.JournalSize(), "failed journal must report 0 on-disk bytes after permanent materialization")
	decodedAfterFail := NewActivityTree(activityTreeInsertTestValidator{}, nil, "security_profile")
	ProtoDecodeActivityTree(decodedAfterFail, protoAfterFail)
	leafAfterFail := decodedAfterFail.ProcessNodes[0].Files["var"].Children["lib"].Children["app"].Children["state.db"]
	require.NotNil(t, leafAfterFail)
	assert.Len(t, leafAfterFail.MatchedRules, 31, "frame buffered before async write error must still be recovered from fallbackBuf")

	// Simulate async pending buffer cap backpressure on a second tree and verify two consecutive ToProto calls preserve all payloads.
	bpTree := NewActivityTree(activityTreeInsertTestValidator{}, NewPathsReducer(), "security_profile")
	require.NoError(t, bpTree.EnableJournal(t.TempDir(), "bp-fallback:v1", 8<<20))
	t.Cleanup(func() { _ = bpTree.CloseJournal() })

	_, _, _, err = bpTree.Insert(fEv, true, "v1", Runtime, nil)
	require.NoError(t, err)
	require.NoError(t, bpTree.Stats.journal.flushAsync())

	bpTree.Stats.journal.ioMu.Lock()
	bpTree.Stats.journal.flushing = true
	bpTree.Stats.journal.pendingBuf = make([]byte, journalMaxAsyncPendingBytes)
	bpTree.Stats.journal.ioMu.Unlock()

	fEvBP := newJournalTestFileOpenEvent(worker, "/var/lib/app/backpressure.db", syscall.O_RDONLY, syscall.S_IFREG|0644, ts.Add(2*time.Minute))
	fEvBP.Open.File.PkgName = "bp-pkg"
	fEvBP.Rules = []*model.MatchedRule{{RuleID: "bp_rule", RuleVersion: "1.0", PolicyName: "default", PolicyVersion: "1"}}
	_, _, _, err = bpTree.Insert(fEvBP, true, "v1", Runtime, nil)
	require.NoError(t, err)
	assert.False(t, bpTree.HasJournal(), "hitting async pending buffer cap while flushing must transition journal to failed in-memory fallback")

	bpTree.Stats.journal.ioMu.Lock()
	bpTree.Stats.journal.flushing = false
	bpTree.Stats.journal.pendingBuf = nil
	bpTree.Stats.journal.ioMu.Unlock()

	for pass := 0; pass < 2; pass++ {
		bpProto := ToProto(bpTree)
		bpDecoded := NewActivityTree(activityTreeInsertTestValidator{}, nil, "security_profile")
		ProtoDecodeActivityTree(bpDecoded, bpProto)
		bpLeaf := bpDecoded.ProcessNodes[0].Files["var"].Children["lib"].Children["app"].Children["backpressure.db"]
		require.NotNil(t, bpLeaf)
		require.NotNil(t, bpLeaf.File)
		assert.Equal(t, "bp-pkg", bpLeaf.File.PkgName)
		require.Len(t, bpLeaf.MatchedRules, 1)
		assert.Equal(t, "bp_rule", bpLeaf.MatchedRules[0].RuleID)
	}
}

func TestActivityTreeJournal_ProcessLiveFieldPreservationSymmetry(t *testing.T) {
	jTree := NewActivityTree(activityTreeInsertTestValidator{}, NewPathsReducer(), "security_profile")
	require.NoError(t, jTree.EnableJournal(t.TempDir(), "proc-symmetry:v1", 0))
	t.Cleanup(func() { _ = jTree.CloseJournal() })

	shim := newJournalTestProcessEntry(1, "/usr/bin/containerd-shim-runc-v2", "containerd-shim-runc-v2", "containerd-shim-runc-v2", nil, nil, nil)
	worker := newJournalTestProcessEntry(100, "/usr/bin/app", "app", "app", []string{"app", "--serve"}, []string{"SECRET=1"}, shim)
	ts := time.Unix(1700000000, 0)

	fEv := newJournalTestFileOpenEvent(worker, "/etc/app.conf", syscall.O_RDONLY, syscall.S_IFREG|0644, ts)
	_, _, _, err := jTree.Insert(fEv, true, "v1", Runtime, nil)
	require.NoError(t, err)

	require.Len(t, jTree.ProcessNodes, 1)
	pn := jTree.ProcessNodes[0]
	pn.Process.PPid = 4242
	pn.Process.Cookie = 987654321
	pn.Process.ExitTime = ts.Add(10 * time.Second)
	pn.Process.IsExecExec = true

	jTree.WithMaterializedTree(func() {
		assert.Equal(t, uint32(4242), pn.Process.PPid, "live PPid must not be clobbered by Materialize")
		assert.Equal(t, uint64(987654321), pn.Process.Cookie, "live Cookie must not be clobbered by Materialize")
		assert.Equal(t, ts.Add(10*time.Second), pn.Process.ExitTime)
		assert.True(t, pn.Process.IsExecExec)
		assert.Equal(t, []string{"SECRET=1"}, pn.Process.Envs)
	})
}
