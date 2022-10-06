// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests
// +build functionaltests

package tests

import (
	"fmt"
	"os"
	"path"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestDentryResolutionERPC(t *testing.T) {
	// generate a basename up to the current limit of the agent
	var basename string
	for i := 0; i < model.MaxSegmentLength; i++ {
		basename += "a"
	}
	rule := &rules.RuleDefinition{
		ID:         "test_erpc_rule",
		Expression: fmt.Sprintf(`open.file.path == "{{.Root}}/%s" && open.flags & O_CREAT != 0`, basename),
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{disableMapDentryResolution: true})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	test.WaitSignal(t, func() error {
		testFile, _, err := test.Create(basename)
		if err != nil {
			return err
		}
		return os.Remove(testFile)
	}, func(event *sprobe.Event, rule *rules.Rule) {
		assertTriggeredRule(t, rule, "test_erpc_rule")
	})

	test.module.SendStats()

	key := metrics.MetricDentryResolverHits + ":" + metrics.ERPCTag
	assert.NotEmpty(t, test.statsdClient.Get(key))

	key = metrics.MetricDentryResolverHits + ":" + metrics.KernelMapsTag
	assert.Empty(t, test.statsdClient.Get(key))
}

func TestDentryResolutionMap(t *testing.T) {
	// generate a basename up to the current limit of the agent
	var basename string
	for i := 0; i < model.MaxSegmentLength; i++ {
		basename += "a"
	}
	rule := &rules.RuleDefinition{
		ID:         "test_map_rule",
		Expression: fmt.Sprintf(`open.file.path == "{{.Root}}/%s" && open.flags & O_CREAT != 0`, basename),
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{disableERPCDentryResolution: true})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	test.WaitSignal(t, func() error {
		testFile, _, err := test.Create(basename)
		if err != nil {
			return err
		}
		return os.Remove(testFile)
	}, func(event *sprobe.Event, rule *rules.Rule) {
		assertTriggeredRule(t, rule, "test_map_rule")
	})

	test.module.SendStats()

	key := metrics.MetricDentryResolverHits + ":" + metrics.KernelMapsTag
	assert.NotEmpty(t, test.statsdClient.Get(key))

	key = metrics.MetricDentryResolverHits + ":" + metrics.ERPCTag
	assert.Empty(t, test.statsdClient.Get(key))
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
	}, func(event *sprobe.Event, _ *rules.Rule) {
		mountID = event.Open.File.MountID
		inode = event.Open.File.Inode
		pathID = event.Open.File.PathID
	})
	if err != nil {
		b.Fatal(err)
	}

	// create a new dentry resolver to avoid concurrent map access errors
	resolver, err := sprobe.NewDentryResolver(test.probe)
	if err != nil {
		b.Fatal(err)
	}

	if err := resolver.Start(test.probe); err != nil {
		b.Fatal(err)
	}
	name, err := resolver.GetNameFromERPC(mountID, inode, pathID)
	if err != nil {
		b.Fatal(err)
	}
	b.Log(name)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		name, err = resolver.GetNameFromERPC(mountID, inode, pathID)
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
	}, func(event *sprobe.Event, _ *rules.Rule) {
		mountID = event.Open.File.MountID
		inode = event.Open.File.Inode
		pathID = event.Open.File.PathID
	})
	if err != nil {
		b.Fatal(err)
	}

	// create a new dentry resolver to avoid concurrent map access errors
	resolver, err := sprobe.NewDentryResolver(test.probe)
	if err != nil {
		b.Fatal(err)
	}

	if err := resolver.Start(test.probe); err != nil {
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
	}, func(event *sprobe.Event, _ *rules.Rule) {
		mountID = event.Open.File.MountID
		inode = event.Open.File.Inode
		pathID = event.Open.File.PathID
	})
	if err != nil {
		b.Fatal(err)
	}

	// create a new dentry resolver to avoid concurrent map access errors
	resolver, err := sprobe.NewDentryResolver(test.probe)
	if err != nil {
		b.Fatal(err)
	}

	if err := resolver.Start(test.probe); err != nil {
		b.Fatal(err)
	}
	name, err := resolver.GetNameFromMap(mountID, inode, pathID)
	if err != nil {
		b.Fatal(err)
	}
	b.Log(name)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		name, err = resolver.GetNameFromMap(mountID, inode, pathID)
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
	}, func(event *sprobe.Event, _ *rules.Rule) {
		mountID = event.Open.File.MountID
		inode = event.Open.File.Inode
		pathID = event.Open.File.PathID
	})
	if err != nil {
		b.Fatal(err)
	}

	// create a new dentry resolver to avoid concurrent map access errors
	resolver, err := sprobe.NewDentryResolver(test.probe)
	if err != nil {
		b.Fatal(err)
	}

	if err := resolver.Start(test.probe); err != nil {
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
