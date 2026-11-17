// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet || docker

// Package container provides container-based log launchers
package container

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/logs-library/pipeline"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditorMock "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers/container/tailerfactory"
	containertailers "github.com/DataDog/datadog-agent/pkg/logs/launchers/container/tailerfactory/tailers"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// testFactory is a test implementation of tailerfactory.Factory.
type testFactory struct {
	makeTailer func(*sources.LogSource) (tailerfactory.Tailer, error)
}

// MakeTailer implements tailerfactory.Factory#MakeTailer.
func (tf *testFactory) MakeTailer(source *sources.LogSource) (tailerfactory.Tailer, error) {
	return tf.makeTailer(source)
}

func TestStartStop(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	l := NewLauncher(nil, option.None[workloadmeta.Component](), fakeTagger)

	sp := launchers.NewMockSourceProvider()
	pl := pipeline.NewMockProvider()
	reg := auditorMock.NewMockRegistry()
	tailerTracker := tailers.NewTailerTracker()
	l.Start(sp, pl, reg, tailerTracker)

	require.NotNil(t, l.cancel)
	require.NotNil(t, l.stopped)

	l.Stop()

	require.Nil(t, l.cancel)
	require.Nil(t, l.stopped)
}

func TestAddsRemovesSource(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	l := NewLauncher(nil, option.None[workloadmeta.Component](), fakeTagger)
	l.tailerFactory = &testFactory{
		makeTailer: func(source *sources.LogSource) (tailerfactory.Tailer, error) {
			return &tailerfactory.TestTailer{Name: source.Name}, nil
		},
	}
	addedSources := make(chan *sources.LogSource, 1)
	removedSources := make(chan *sources.LogSource, 1)

	source := sources.NewLogSource("test-source", &config.LogsConfig{
		Type:       "docker",
		Identifier: "abc",
	})

	addedSources <- source
	require.True(t, l.loop(context.Background(), addedSources, removedSources))

	tailer := l.tailers[source].(*tailerfactory.TestTailer)
	require.Equal(t, "test-source", tailer.Name)
	require.True(t, tailer.Started)

	removedSources <- source
	require.True(t, l.loop(context.Background(), addedSources, removedSources))

	require.Nil(t, l.tailers[source])
	require.True(t, tailer.Stopped)
}

func TestCannotMakeTailer(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	l := NewLauncher(nil, option.None[workloadmeta.Component](), fakeTagger)
	l.tailerFactory = &testFactory{
		makeTailer: func(_ *sources.LogSource) (tailerfactory.Tailer, error) {
			return nil, errors.New("uhoh")
		},
	}
	addedSources := make(chan *sources.LogSource, 1)
	removedSources := make(chan *sources.LogSource, 1)

	source := sources.NewLogSource("test-source", &config.LogsConfig{
		Type:       "docker",
		Identifier: "abc",
	})

	addedSources <- source
	require.True(t, l.loop(context.Background(), addedSources, removedSources))
	require.Nil(t, l.tailers[source])
	require.Equal(t, "Error: uhoh", source.Status().GetError())
}

func TestCannotStartTailer(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	l := NewLauncher(nil, option.None[workloadmeta.Component](), fakeTagger)
	l.tailerFactory = &testFactory{
		makeTailer: func(source *sources.LogSource) (tailerfactory.Tailer, error) {
			return &tailerfactory.TestTailer{Name: source.Name, StartError: true}, nil
		},
	}
	addedSources := make(chan *sources.LogSource, 1)
	removedSources := make(chan *sources.LogSource, 1)

	source := sources.NewLogSource("test-source", &config.LogsConfig{
		Type:       "docker",
		Identifier: "abc",
	})

	addedSources <- source
	require.True(t, l.loop(context.Background(), addedSources, removedSources))
	require.Nil(t, l.tailers[source])
	require.Equal(t, "Error: uhoh", source.Status().GetError())
}

// A tailer adding and removing a file source from the launcher loop, like
// tailers.WrappedSource does, must not deadlock the launcher.
func TestChildFileSourceAddedFromLoop(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	logSources := sources.NewLogSources()

	// stand-in for the file launcher
	fileDone := make(chan struct{})
	fileAdded, fileRemoved := logSources.SubscribeForType(config.FileType, fileDone, fileDone)

	l := NewLauncher(logSources, option.None[workloadmeta.Component](), fakeTagger)
	l.tailerFactory = &testFactory{
		makeTailer: func(source *sources.LogSource) (tailerfactory.Tailer, error) {
			return &containertailers.WrappedSource{
				Source: sources.NewLogSource(source.Name, &config.LogsConfig{
					Type:       config.FileType,
					Path:       "/var/log/pods/abc/0.log",
					Identifier: source.Config.Identifier,
				}),
				Sources: logSources,
			}, nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	l.cancel = cancel
	l.stopped = make(chan struct{})
	go l.run(ctx, logSources)
	defer l.Stop()
	// runs before l.Stop, so that stopping the launcher never waits on this
	// stand-in
	defer close(fileDone)

	receive := func(ch chan *sources.LogSource) *sources.LogSource {
		t.Helper()
		select {
		case source := <-ch:
			return source
		case <-time.After(10 * time.Second):
			require.FailNow(t, "timed out: the container launcher is blocked")
			return nil
		}
	}

	parent := sources.NewLogSource("container_collect_all", &config.LogsConfig{Type: "docker", Identifier: "abc"})
	logSources.AddSource(parent)
	child := receive(fileAdded)
	require.Equal(t, "abc", child.Config.Identifier)

	logSources.RemoveSource(parent)
	require.Same(t, child, receive(fileRemoved))
	require.Empty(t, logSources.GetSources())
}

// Removing a source right after adding it, without waiting for the launcher to
// process the addition, must stop its tailer and remove its child file source.
func TestAddRemoveSourceWithoutWaiting(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	logSources := sources.NewLogSources()

	// stand-in for the file launcher: counts the file sources still added, and
	// reports that count when the child file source of "first" or "last" is
	// added. The launcher adds and removes child file sources synchronously and
	// in order, so all the earlier ones were received by then.
	fileDone := make(chan struct{})
	fileAdded, fileRemoved := logSources.SubscribeForType(config.FileType, fileDone, fileDone)
	activeWhenAdded := map[string]chan int{"first": make(chan int, 1), "last": make(chan int, 1)}
	go func() {
		active := 0
		for {
			select {
			case source := <-fileAdded:
				active++
				if ch, ok := activeWhenAdded[source.Config.Identifier]; ok {
					ch <- active
				}
			case <-fileRemoved:
				active--
			case <-fileDone:
				return
			}
		}
	}()

	l := NewLauncher(logSources, option.None[workloadmeta.Component](), fakeTagger)
	l.tailerFactory = &testFactory{
		makeTailer: func(source *sources.LogSource) (tailerfactory.Tailer, error) {
			return &containertailers.WrappedSource{
				Source: sources.NewLogSource(source.Name, &config.LogsConfig{
					Type:       config.FileType,
					Path:       "/var/log/pods/" + source.Config.Identifier + "/0.log",
					Identifier: source.Config.Identifier,
				}),
				Sources: logSources,
			}, nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	l.cancel = cancel
	l.stopped = make(chan struct{})
	go l.run(ctx, logSources)
	defer l.Stop()
	defer close(fileDone) // runs before l.Stop

	waitActive := func(id string, expected int, msg string) {
		t.Helper()
		select {
		case active := <-activeWhenAdded[id]:
			require.Equal(t, expected, active, msg)
		case <-time.After(10 * time.Second):
			require.FailNow(t, "timed out: the container launcher is blocked")
		}
	}

	// Wait for the launcher to process a first source: sources existing when
	// the launcher subscribes are replayed asynchronously, so their removal
	// could otherwise be received before their addition.
	first := sources.NewLogSource("container_collect_all", &config.LogsConfig{Type: "docker", Identifier: "first"})
	logSources.AddSource(first)
	waitActive("first", 1, "unexpected file sources")
	logSources.RemoveSource(first)

	const n = 500
	for i := range n {
		parent := sources.NewLogSource("container_collect_all", &config.LogsConfig{Type: "docker", Identifier: fmt.Sprintf("c%d", i)})
		logSources.AddSource(parent)
		logSources.RemoveSource(parent)
	}

	last := sources.NewLogSource("container_collect_all", &config.LogsConfig{Type: "docker", Identifier: "last"})
	logSources.AddSource(last)
	waitActive("last", 1, "file sources left behind by removed containers")
	require.Equal(t, []*sources.LogSource{last}, nonFileSources(logSources))
}

func nonFileSources(logSources *sources.LogSources) []*sources.LogSource {
	var res []*sources.LogSource
	for _, source := range logSources.GetSources() {
		if source.Config.Type != config.FileType {
			res = append(res, source)
		}
	}
	return res
}
