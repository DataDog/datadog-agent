// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package tailerfactory

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func TestMakeTailerFile(t *testing.T) {
	useFile := func(*sources.LogSource) bool { return true }
	makeFileTailer := func(*sources.LogSource) (Tailer, error) { return &TestTailer{}, nil }
	makeSocketTailer := func(*sources.LogSource) (Tailer, error) { panic("shouldn't be called") }

	tailer, err := (&factory{}).makeTailer(&sources.LogSource{}, useFile, makeFileTailer, makeSocketTailer)
	require.NoError(t, err)
	require.NotNil(t, tailer)
}

func TestMakeTailerFileFallback(t *testing.T) {
	useFile := func(*sources.LogSource) bool { return true }
	makeFileTailer := func(*sources.LogSource) (Tailer, error) { return nil, errors.New("uhoh") }
	makeSocketTailer := func(*sources.LogSource) (Tailer, error) { return &TestTailer{}, nil }

	source := &sources.LogSource{Messages: config.NewMessages()}
	tailer, err := (&factory{}).makeTailer(source, useFile, makeFileTailer, makeSocketTailer)
	messages := source.Messages.GetMessages()

	require.NoError(t, err)
	require.NotNil(t, tailer)
	require.NotNil(t, messages)
	require.Contains(t, messages, "The log file tailer could not be made, falling back to socket")
}

func TestMakeTailerFileFallbackFailsToo(t *testing.T) {
	useFile := func(*sources.LogSource) bool { return true }
	makeFileTailer := func(*sources.LogSource) (Tailer, error) { return nil, errors.New("uhoh1") }
	makeSocketTailer := func(*sources.LogSource) (Tailer, error) { return nil, errors.New("uhoh2") }
	source := &sources.LogSource{Messages: config.NewMessages()}

	tailer, err := (&factory{}).makeTailer(source, useFile, makeFileTailer, makeSocketTailer)
	require.ErrorContains(t, err, "uhoh2")
	require.Nil(t, tailer)
}

func TestMakeTailerSocket(t *testing.T) {
	useFile := func(*sources.LogSource) bool { return false }
	makeFileTailer := func(*sources.LogSource) (Tailer, error) { panic("shouldn't be called") }
	makeSocketTailer := func(*sources.LogSource) (Tailer, error) { return &TestTailer{}, nil }

	tailer, err := (&factory{}).makeTailer(&sources.LogSource{}, useFile, makeFileTailer, makeSocketTailer)
	require.NoError(t, err)
	require.NotNil(t, tailer)
}

func TestMakeTailerSocketFallback(t *testing.T) {
	useFile := func(*sources.LogSource) bool { return false }
	makeFileTailer := func(*sources.LogSource) (Tailer, error) { return &TestTailer{}, nil }
	makeSocketTailer := func(*sources.LogSource) (Tailer, error) { return nil, errors.New("uhoh") }

	source := &sources.LogSource{Messages: config.NewMessages()}
	tailer, err := (&factory{}).makeTailer(source, useFile, makeFileTailer, makeSocketTailer)
	require.NoError(t, err)
	require.NotNil(t, tailer)
	messages := source.Messages.GetMessages()
	require.NotNil(t, messages)

	require.NotNil(t, source.Messages.GetMessages())
	require.Contains(t, messages, "The socket tailer could not be made, falling back to file")
}

func TestMakeTailerSocketFallbackFailsToo(t *testing.T) {
	useFile := func(*sources.LogSource) bool { return false }
	makeFileTailer := func(*sources.LogSource) (Tailer, error) { return nil, errors.New("uhoh2") }
	makeSocketTailer := func(*sources.LogSource) (Tailer, error) { return nil, errors.New("uhoh1") }
	source := &sources.LogSource{Messages: config.NewMessages()}

	tailer, err := (&factory{}).makeTailer(source, useFile, makeFileTailer, makeSocketTailer)
	require.ErrorContains(t, err, "uhoh2")
	require.Nil(t, tailer)
}
