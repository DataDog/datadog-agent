// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker
// +build docker

package container

import (
	"context"
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers/container/tailerfactory"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/stretchr/testify/require"
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
	l := NewLauncher(nil)

	sp := launchers.NewMockSourceProvider()
	pl := pipeline.NewMockProvider()
	reg := auditor.New("/run", "agent", 0, nil)
	l.Start(sp, pl, reg)

	require.NotNil(t, l.cancel)
	require.NotNil(t, l.stopped)

	l.Stop()

	require.Nil(t, l.cancel)
	require.Nil(t, l.stopped)
}

func TestAddsRemovesSource(t *testing.T) {
	l := NewLauncher(nil)
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
	l := NewLauncher(nil)
	l.tailerFactory = &testFactory{
		makeTailer: func(source *sources.LogSource) (tailerfactory.Tailer, error) {
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
	require.Equal(t, "Error: uhoh", source.Status.GetError())
}

func TestCannotStartTailer(t *testing.T) {
	l := NewLauncher(nil)
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
	require.Equal(t, "Error: uhoh", source.Status.GetError())
}
