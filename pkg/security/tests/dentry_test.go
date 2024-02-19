// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"os"
	"path"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/dentry"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestDentryPathERPC(t *testing.T) {
	SkipIfNotAvailable(t)

	// generate a basename up to the current limit of the agent
	var basename string
	for i := 0; i < model.MaxSegmentLength; i++ {
		basename += "a"
	}
	rule := &rules.RuleDefinition{
		ID:         "test_erpc_path_rule",
		Expression: `open.flags & (O_CREAT|O_NOCTTY|O_NOFOLLOW) != 0 && process.file.name == "testsuite"`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, withStaticOpts(testOpts{disableMapDentryResolution: true}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	p, ok := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		t.Skip("not supported")
	}

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
		file, err := os.OpenFile(testFile, os.O_CREATE|unix.O_NOCTTY|unix.O_NOFOLLOW, 0666)
		if err != nil {
			return err
		}
		file.Close()
		return nil
	}, func(event *model.Event, rule *rules.Rule) {
		assertTriggeredRule(t, rule, "test_erpc_path_rule")

		basename := path.Base(testFile)

		res, err := p.Resolvers.DentryResolver.Resolve(event.Open.File.PathKey, true)
		assert.Nil(test.t, err)
		assert.Equal(test.t, basename, path.Base(res))

		// check that the path is now available from the cache
		res, err = p.Resolvers.DentryResolver.ResolveFromCache(event.Open.File.PathKey)
		assert.Nil(test.t, err)
		assert.Equal(test.t, basename, path.Base(res))

		// check stats
		test.eventMonitor.SendStats()

		key := metrics.MetricDentryResolverHits + ":" + metrics.ERPCTag
		assert.NotEmpty(t, test.statsdClient.Get(key))

		key = metrics.MetricDentryResolverHits + ":" + metrics.KernelMapsTag
		assert.Empty(t, test.statsdClient.Get(key))
	})
}

func TestDentryPathMap(t *testing.T) {
	SkipIfNotAvailable(t)

	// generate a basename up to the current limit of the agent
	var basename string
	for i := 0; i < model.MaxSegmentLength; i++ {
		basename += "a"
	}
	rule := &rules.RuleDefinition{
		ID:         "test_map_path_rule",
		Expression: `open.flags & (O_CREAT|O_NOCTTY|O_NOFOLLOW) != 0 && process.file.name == "testsuite"`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, withStaticOpts(testOpts{disableERPCDentryResolution: true}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	p, ok := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		t.Skip("not supported")
	}

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
		file, err := os.OpenFile(testFile, os.O_CREATE|unix.O_NOCTTY|unix.O_NOFOLLOW, 0666)
		if err != nil {
			return err
		}
		file.Close()
		return nil
	}, func(event *model.Event, rule *rules.Rule) {
		assertTriggeredRule(t, rule, "test_map_path_rule")

		basename := path.Base(testFile)

		res, err := p.Resolvers.DentryResolver.Resolve(event.Open.File.PathKey, true)
		assert.Nil(test.t, err)
		assert.Equal(test.t, basename, path.Base(res))

		// check that the path is now available from the cache
		res, err = p.Resolvers.DentryResolver.ResolveFromCache(event.Open.File.PathKey)
		assert.Nil(test.t, err)
		assert.Equal(test.t, basename, path.Base(res))

		// check stats
		test.eventMonitor.SendStats()

		key := metrics.MetricDentryResolverHits + ":" + metrics.ERPCTag
		assert.Empty(t, test.statsdClient.Get(key))

		key = metrics.MetricDentryResolverHits + ":" + metrics.KernelMapsTag
		assert.NotEmpty(t, test.statsdClient.Get(key))
	})
}

func TestDentryName(t *testing.T) {
	SkipIfNotAvailable(t)

	// generate a basename up to the current limit of the agent
	var basename string
	for i := 0; i < model.MaxSegmentLength; i++ {
		basename += "a"
	}
	rule := &rules.RuleDefinition{
		ID:         "test_dentry_name_rule",
		Expression: `open.flags & (O_CREAT|O_NOCTTY|O_NOFOLLOW) != 0 && process.file.name == "testsuite"`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	p, ok := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		t.Skip("not supported")
	}

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
		file, err := os.OpenFile(testFile, os.O_CREATE|unix.O_NOCTTY|unix.O_NOFOLLOW, 0666)
		if err != nil {
			return err
		}
		file.Close()
		return nil
	}, func(event *model.Event, rule *rules.Rule) {
		assertTriggeredRule(t, rule, "test_dentry_name_rule")

		basename := path.Base(testFile)

		// check that the path is now available from the cache
		res := p.Resolvers.DentryResolver.ResolveName(event.Open.File.PathKey)
		assert.Equal(test.t, basename, res)

		// check that the path is now available from the cache
		res, err = p.Resolvers.DentryResolver.ResolveNameFromCache(event.Open.File.PathKey)
		assert.Nil(test.t, err)
		assert.Equal(test.t, basename, path.Base(res))
	})
}

func BenchmarkERPCDentryResolutionPath(b *testing.B) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path == "{{.Root}}/aa/bb/cc/dd/ee" && open.flags & O_CREAT != 0`,
	}

	test, err := newTestModule(b, nil, []*rules.RuleDefinition{rule}, withStaticOpts(testOpts{disableMapDentryResolution: true}))
	if err != nil {
		b.Fatal(err)
	}
	defer test.Close()

	p, ok := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		b.Skip("not supported")
	}

	testFile, _, err := test.Path("aa/bb/cc/dd/ee")
	if err != nil {
		b.Fatal(err)
	}
	_ = os.MkdirAll(path.Dir(testFile), 0755)

	defer os.Remove(testFile)

	var pathKey model.PathKey

	err = test.GetSignal(b, func() error {
		fd, err := syscall.Open(testFile, syscall.O_CREAT, 0755)
		if err != nil {
			return err
		}
		return syscall.Close(fd)
	}, func(event *model.Event, _ *rules.Rule) {
		pathKey = event.Open.File.PathKey
	})
	if err != nil {
		b.Fatal(err)
	}

	// create a new dentry resolver to avoid concurrent map access errors
	resolver, err := dentry.NewResolver(test.probe.Config.Probe, test.probe.StatsdClient, p.Erpc)
	if err != nil {
		b.Fatal(err)
	}

	if err := resolver.Start(p.Manager); err != nil {
		b.Fatal(err)
	}
	f, err := resolver.ResolveFromERPC(pathKey, true)
	if err != nil {
		b.Fatal(err)
	}
	b.Log(f)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		f, err := resolver.ResolveFromERPC(pathKey, true)
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

	test, err := newTestModule(b, nil, []*rules.RuleDefinition{rule}, withStaticOpts(testOpts{disableERPCDentryResolution: true}))
	if err != nil {
		b.Fatal(err)
	}
	defer test.Close()

	p, ok := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		b.Skip("not supported")
	}

	testFile, _, err := test.Path("aa/bb/cc/dd/ee")
	if err != nil {
		b.Fatal(err)
	}
	_ = os.MkdirAll(path.Dir(testFile), 0755)

	defer os.Remove(testFile)

	var pathKey model.PathKey

	err = test.GetSignal(b, func() error {
		fd, err := syscall.Open(testFile, syscall.O_CREAT, 0755)
		if err != nil {
			return err
		}
		return syscall.Close(fd)
	}, func(event *model.Event, _ *rules.Rule) {
		pathKey = event.Open.File.PathKey
	})
	if err != nil {
		b.Fatal(err)
	}

	// create a new dentry resolver to avoid concurrent map access errors
	resolver, err := dentry.NewResolver(test.probe.Config.Probe, test.probe.StatsdClient, p.Erpc)
	if err != nil {
		b.Fatal(err)
	}

	if err := resolver.Start(p.Manager); err != nil {
		b.Fatal(err)
	}
	name, err := resolver.ResolveNameFromMap(pathKey)
	if err != nil {
		b.Fatal(err)
	}
	b.Log(name)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		name, err = resolver.ResolveNameFromMap(pathKey)
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

	test, err := newTestModule(b, nil, []*rules.RuleDefinition{rule}, withStaticOpts(testOpts{disableERPCDentryResolution: true}))
	if err != nil {
		b.Fatal(err)
	}
	defer test.Close()

	p, ok := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		b.Skip("not supported")
	}

	testFile, _, err := test.Path("aa/bb/cc/dd/ee")
	if err != nil {
		b.Fatal(err)
	}
	_ = os.MkdirAll(path.Dir(testFile), 0755)

	defer os.Remove(testFile)

	var pathKey model.PathKey
	err = test.GetSignal(b, func() error {
		fd, err := syscall.Open(testFile, syscall.O_CREAT, 0755)
		if err != nil {
			return err
		}
		return syscall.Close(fd)
	}, func(event *model.Event, _ *rules.Rule) {
		pathKey = event.Open.File.PathKey
	})
	if err != nil {
		b.Fatal(err)
	}

	// create a new dentry resolver to avoid concurrent map access errors
	resolver, err := dentry.NewResolver(test.probe.Config.Probe, test.probe.StatsdClient, p.Erpc)
	if err != nil {
		b.Fatal(err)
	}

	if err := resolver.Start(p.Manager); err != nil {
		b.Fatal(err)
	}
	f, err := resolver.ResolveFromMap(pathKey, true)
	if err != nil {
		b.Fatal(err)
	}
	b.Log(f)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		f, err := resolver.ResolveFromMap(pathKey, true)
		if err != nil {
			b.Fatal(err)
		}
		if f[0] == 0 {
			b.Fatal("couldn't resolve file")
		}
	}

	test.Close()
}
