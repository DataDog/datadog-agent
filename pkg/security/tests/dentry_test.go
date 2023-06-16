// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

package tests

import (
	"fmt"
	"os"
	"path"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/dentry"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func validateResolution(test *testModule, event *model.Event, testFile string, pathFnc func(uint32, uint64, uint32, bool) (string, error), parentFnc func(uint32, uint64, uint32) (uint32, uint64, error), nameFnc func(uint32, uint64, uint32) (string, error)) {
	basename := path.Base(testFile)

	// Force an eRPC resolution to refresh the entry with the last generation as lost events may have invalidated the entry
	res, err := pathFnc(event.Open.File.MountID, event.Open.File.Inode, event.Open.File.PathID, true)
	assert.Nil(test.t, err)
	assert.Equal(test.t, basename, path.Base(res))

	// there is a potential race here has a lost event can occur between the two resolutions

	// check that the path is now available from the cache
	res, err = test.probe.GetResolvers().DentryResolver.ResolveFromCache(event.Open.File.MountID, event.Open.File.Inode)
	assert.Nil(test.t, err)
	assert.Equal(test.t, basename, path.Base(res))

	kv, err := kernel.NewKernelVersion()
	assert.Nil(test.t, err)

	// Parent
	expectedInode := getInode(test.t, path.Dir(testFile))

	// the previous path resolution should habe filled the cache
	_, cacheInode, err := test.probe.GetResolvers().DentryResolver.ResolveParentFromCache(event.Open.File.MountID, event.Open.File.Inode)
	assert.Nil(test.t, err)
	assert.NotZero(test.t, cacheInode)

	// on kernel < 5.0 the cache is populated with internal inode of overlayfs. The stat syscall returns the proper inode, that is why the inodes don't match.
	if event.Open.File.Filesystem != model.OverlayFS || kv.Code > kernel.Kernel5_0 {
		assert.Equal(test.t, expectedInode, cacheInode)
	}

	_, inode, err := parentFnc(event.Open.File.MountID, event.Open.File.Inode, event.Open.File.PathID)
	assert.Nil(test.t, err)
	assert.NotZero(test.t, inode)
	assert.Equal(test.t, cacheInode, inode)

	// Basename
	// the previous path resolution should have filled the cache
	expectedName, err := test.probe.GetResolvers().DentryResolver.ResolveNameFromCache(event.Open.File.MountID, event.Open.File.Inode)
	assert.Nil(test.t, err)
	assert.Equal(test.t, expectedName, basename)

	expectedName, err = nameFnc(event.Open.File.MountID, event.Open.File.Inode, event.Open.File.PathID)
	assert.Nil(test.t, err)
	assert.Equal(test.t, expectedName, basename)
}

func TestDentryResolutionERPC(t *testing.T) {
	// generate a basename up to the current limit of the agent
	var basename string
	for i := 0; i < model.MaxSegmentLength; i++ {
		basename += "a"
	}
	rule := &rules.RuleDefinition{
		ID:         "test_erpc_rule",
		Expression: fmt.Sprintf(`open.file.path == "{{.Root}}/parent/%s" && open.flags & O_CREAT != 0`, basename),
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{disableMapDentryResolution: true})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, _, err := test.Path("parent/" + basename)
	if err != nil {
		t.Fatal(err)
	}

	dir := path.Dir(testFile)
	if err := os.MkdirAll(dir, 0777); err != nil {
		t.Fatalf("failed to create directory: %s", err)
	}
	defer os.RemoveAll(dir)

	test.WaitSignal(t, func() error {
		_, err = os.Create(testFile)
		return err
	}, func(event *model.Event, rule *rules.Rule) {
		assertTriggeredRule(t, rule, "test_erpc_rule")

		test.eventMonitor.SendStats()

		key := metrics.MetricDentryResolverHits + ":" + metrics.ERPCTag
		assert.NotEmpty(t, test.statsdClient.Get(key))

		key = metrics.MetricDentryResolverHits + ":" + metrics.KernelMapsTag
		assert.Empty(t, test.statsdClient.Get(key))

		validateResolution(test, event, testFile,
			test.probe.GetResolvers().DentryResolver.ResolveFromERPC,
			test.probe.GetResolvers().DentryResolver.ResolveParentFromERPC,
			test.probe.GetResolvers().DentryResolver.ResolveNameFromERPC,
		)
	})
}

func TestDentryResolutionMap(t *testing.T) {
	// generate a basename up to the current limit of the agent
	var basename string
	for i := 0; i < model.MaxSegmentLength; i++ {
		basename += "a"
	}
	rule := &rules.RuleDefinition{
		ID:         "test_map_rule",
		Expression: fmt.Sprintf(`open.file.path == "{{.Root}}/parent/%s" && open.flags & O_CREAT != 0`, basename),
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{disableERPCDentryResolution: true})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, _, err := test.Path("parent/" + basename)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	dir := path.Dir(testFile)
	if err := os.MkdirAll(dir, 0777); err != nil {
		t.Fatalf("failed to create directory: %s", err)
	}
	defer os.Remove(dir)

	test.WaitSignal(t, func() error {
		_, err := os.Create(testFile)
		if err != nil {
			return err
		}
		return nil
	}, func(event *model.Event, rule *rules.Rule) {
		assertTriggeredRule(t, rule, "test_map_rule")

		test.eventMonitor.SendStats()

		key := metrics.MetricDentryResolverHits + ":" + metrics.KernelMapsTag
		assert.NotEmpty(t, test.statsdClient.Get(key))

		key = metrics.MetricDentryResolverHits + ":" + metrics.ERPCTag
		assert.Empty(t, test.statsdClient.Get(key))

		validateResolution(test, event, testFile,
			test.probe.GetResolvers().DentryResolver.ResolveFromMap,
			test.probe.GetResolvers().DentryResolver.ResolveParentFromMap,
			test.probe.GetResolvers().DentryResolver.ResolveNameFromMap,
		)
	})
}

func BenchmarkERPCDentryResolutionSegment(b *testing.B) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path == "{{.Root}}/aa/bb/cc/dd/ee" && open.flags & O_CREAT != 0`,
	}

	test, err := newTestModule(b, nil, []*rules.RuleDefinition{rule}, testOpts{disableMapDentryResolution: true})
	if err != nil {
		b.Fatal(err)
	}
	defer test.Close()

	testFile, _, err := test.Path("aa/bb/cc/dd/ee")
	if err != nil {
		b.Fatal(err)
	}
	_ = os.MkdirAll(path.Dir(testFile), 0755)

	defer os.Remove(testFile)

	var (
		mountID uint32
		inode   uint64
		pathID  uint32
	)
	err = test.GetSignal(b, func() error {
		fd, err := syscall.Open(testFile, syscall.O_CREAT, 0755)
		if err != nil {
			return err
		}
		return syscall.Close(fd)
	}, func(event *model.Event, _ *rules.Rule) {
		mountID = event.Open.File.MountID
		inode = event.Open.File.Inode
		pathID = event.Open.File.PathID
	})
	if err != nil {
		b.Fatal(err)
	}

	// create a new dentry resolver to avoid concurrent map access errors
	resolver, err := dentry.NewResolver(test.probe.Config.Probe, test.probe.StatsdClient, test.probe.Erpc)
	if err != nil {
		b.Fatal(err)
	}

	if err := resolver.Start(test.probe.Manager); err != nil {
		b.Fatal(err)
	}
	name, err := resolver.ResolveNameFromERPC(mountID, inode, pathID)
	if err != nil {
		b.Fatal(err)
	}
	b.Log(name)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		name, err = resolver.ResolveNameFromERPC(mountID, inode, pathID)
		if err != nil {
			b.Fatal(err)
		}
		if len(name) == 0 || len(name) > 0 && name[0] == 0 {
			b.Log("couldn't resolve segment")
		}
	}

	test.Close()
}

func BenchmarkERPCDentryResolutionPath(b *testing.B) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path == "{{.Root}}/aa/bb/cc/dd/ee" && open.flags & O_CREAT != 0`,
	}

	test, err := newTestModule(b, nil, []*rules.RuleDefinition{rule}, testOpts{disableMapDentryResolution: true})
	if err != nil {
		b.Fatal(err)
	}
	defer test.Close()

	testFile, _, err := test.Path("aa/bb/cc/dd/ee")
	if err != nil {
		b.Fatal(err)
	}
	_ = os.MkdirAll(path.Dir(testFile), 0755)

	defer os.Remove(testFile)

	var (
		mountID uint32
		inode   uint64
		pathID  uint32
	)
	err = test.GetSignal(b, func() error {
		fd, err := syscall.Open(testFile, syscall.O_CREAT, 0755)
		if err != nil {
			return err
		}
		return syscall.Close(fd)
	}, func(event *model.Event, _ *rules.Rule) {
		mountID = event.Open.File.MountID
		inode = event.Open.File.Inode
		pathID = event.Open.File.PathID
	})
	if err != nil {
		b.Fatal(err)
	}

	// create a new dentry resolver to avoid concurrent map access errors
	resolver, err := dentry.NewResolver(test.probe.Config.Probe, test.probe.StatsdClient, test.probe.Erpc)
	if err != nil {
		b.Fatal(err)
	}

	if err := resolver.Start(test.probe.Manager); err != nil {
		b.Fatal(err)
	}
	f, err := resolver.ResolveFromERPC(mountID, inode, pathID, true)
	if err != nil {
		b.Fatal(err)
	}
	b.Log(f)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		f, err := resolver.ResolveFromERPC(mountID, inode, pathID, true)
		if err != nil {
			b.Fatal(err)
		}
		if len(f) == 0 || len(f) > 0 && f[0] == 0 {
			b.Log("couldn't resolve path")
		}
	}

	test.Close()
}

func BenchmarkMapDentryResolutionSegment(b *testing.B) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path == "{{.Root}}/aa/bb/cc/dd/ee" && open.flags & O_CREAT != 0`,
	}

	test, err := newTestModule(b, nil, []*rules.RuleDefinition{rule}, testOpts{disableERPCDentryResolution: true})
	if err != nil {
		b.Fatal(err)
	}
	defer test.Close()

	testFile, _, err := test.Path("aa/bb/cc/dd/ee")
	if err != nil {
		b.Fatal(err)
	}
	_ = os.MkdirAll(path.Dir(testFile), 0755)

	defer os.Remove(testFile)

	var (
		mountID uint32
		inode   uint64
		pathID  uint32
	)
	err = test.GetSignal(b, func() error {
		fd, err := syscall.Open(testFile, syscall.O_CREAT, 0755)
		if err != nil {
			return err
		}
		return syscall.Close(fd)
	}, func(event *model.Event, _ *rules.Rule) {
		mountID = event.Open.File.MountID
		inode = event.Open.File.Inode
		pathID = event.Open.File.PathID
	})
	if err != nil {
		b.Fatal(err)
	}

	// create a new dentry resolver to avoid concurrent map access errors
	resolver, err := dentry.NewResolver(test.probe.Config.Probe, test.probe.StatsdClient, test.probe.Erpc)
	if err != nil {
		b.Fatal(err)
	}

	if err := resolver.Start(test.probe.Manager); err != nil {
		b.Fatal(err)
	}
	name, err := resolver.ResolveNameFromMap(mountID, inode, pathID)
	if err != nil {
		b.Fatal(err)
	}
	b.Log(name)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		name, err = resolver.ResolveNameFromMap(mountID, inode, pathID)
		if err != nil {
			b.Fatal(err)
		}
		if len(name) == 0 || len(name) > 0 && name[0] == 0 {
			b.Fatal("couldn't resolve segment")
		}
	}

	test.Close()
}

func BenchmarkMapDentryResolutionPath(b *testing.B) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path == "{{.Root}}/aa/bb/cc/dd/ee" && open.flags & O_CREAT != 0`,
	}

	test, err := newTestModule(b, nil, []*rules.RuleDefinition{rule}, testOpts{disableERPCDentryResolution: true})
	if err != nil {
		b.Fatal(err)
	}
	defer test.Close()

	testFile, _, err := test.Path("aa/bb/cc/dd/ee")
	if err != nil {
		b.Fatal(err)
	}
	_ = os.MkdirAll(path.Dir(testFile), 0755)

	defer os.Remove(testFile)

	var (
		mountID uint32
		inode   uint64
		pathID  uint32
	)
	err = test.GetSignal(b, func() error {
		fd, err := syscall.Open(testFile, syscall.O_CREAT, 0755)
		if err != nil {
			return err
		}
		return syscall.Close(fd)
	}, func(event *model.Event, _ *rules.Rule) {
		mountID = event.Open.File.MountID
		inode = event.Open.File.Inode
		pathID = event.Open.File.PathID
	})
	if err != nil {
		b.Fatal(err)
	}

	// create a new dentry resolver to avoid concurrent map access errors
	resolver, err := dentry.NewResolver(test.probe.Config.Probe, test.probe.StatsdClient, test.probe.Erpc)
	if err != nil {
		b.Fatal(err)
	}

	if err := resolver.Start(test.probe.Manager); err != nil {
		b.Fatal(err)
	}
	f, err := resolver.ResolveFromMap(mountID, inode, pathID, true)
	if err != nil {
		b.Fatal(err)
	}
	b.Log(f)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		f, err := resolver.ResolveFromMap(mountID, inode, pathID, true)
		if err != nil {
			b.Fatal(err)
		}
		if f[0] == 0 {
			b.Fatal("couldn't resolve file")
		}
	}

	test.Close()
}
