// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet || docker

package tailerfactory

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func TestMakeTailerFile(t *testing.T) {
	whichTailer := func(*sources.LogSource) whichTailer { return file }
	makeFileTailer := func(*sources.LogSource) (Tailer, error) { return &TestTailer{}, nil }
	makeSocketTailer := func(*sources.LogSource) (Tailer, error) { panic("socket shouldn't be called") }
	makeAPITailer := func(*sources.LogSource) (Tailer, error) { panic("api shouldn't be called") }

	tailer, err := (&factory{}).makeTailer(&sources.LogSource{}, whichTailer, makeFileTailer, makeSocketTailer, makeAPITailer)
	require.NoError(t, err)
	require.NotNil(t, tailer)
}

func TestMakeTailerFileFallback(t *testing.T) {
	whichTailer := func(*sources.LogSource) whichTailer { return file }
	makeFileTailer := func(*sources.LogSource) (Tailer, error) { return nil, errors.New("uhoh") }
	makeSocketTailer := func(*sources.LogSource) (Tailer, error) { return &TestTailer{}, nil }
	makeAPITailer := func(*sources.LogSource) (Tailer, error) { panic("api shouldn't be called") }

	source := &sources.LogSource{Messages: config.NewMessages()}
	tailer, err := (&factory{}).makeTailer(source, whichTailer, makeFileTailer, makeSocketTailer, makeAPITailer)
	messages := source.Messages.GetMessages()

	require.NoError(t, err)
	require.NotNil(t, tailer)
	require.NotNil(t, messages)
	require.Contains(t, messages, "The log file tailer could not be made, falling back to socket")
}

func TestMakeTailerFileFallbackFailsToo(t *testing.T) {
	whichTailer := func(*sources.LogSource) whichTailer { return file }
	makeFileTailer := func(*sources.LogSource) (Tailer, error) { return nil, errors.New("uhoh1") }
	makeSocketTailer := func(*sources.LogSource) (Tailer, error) { return nil, errors.New("uhoh2") }
	makeAPITailer := func(*sources.LogSource) (Tailer, error) { panic("api shouldn't be called") }
	source := &sources.LogSource{Messages: config.NewMessages()}

	tailer, err := (&factory{}).makeTailer(source, whichTailer, makeFileTailer, makeSocketTailer, makeAPITailer)
	require.ErrorContains(t, err, "uhoh2")
	require.Nil(t, tailer)
}

func TestMakeTailerSocket(t *testing.T) {
	whichTailer := func(*sources.LogSource) whichTailer { return socket }
	makeFileTailer := func(*sources.LogSource) (Tailer, error) { panic("shouldn't be called") }
	makeSocketTailer := func(*sources.LogSource) (Tailer, error) { return &TestTailer{}, nil }
	makeAPITailer := func(*sources.LogSource) (Tailer, error) { panic("api shouldn't be called") }

	tailer, err := (&factory{}).makeTailer(&sources.LogSource{}, whichTailer, makeFileTailer, makeSocketTailer, makeAPITailer)
	require.NoError(t, err)
	require.NotNil(t, tailer)
}

func TestMakeTailerSocketFallback(t *testing.T) {
	whichTailer := func(*sources.LogSource) whichTailer { return socket }
	makeFileTailer := func(*sources.LogSource) (Tailer, error) { return &TestTailer{}, nil }
	makeSocketTailer := func(*sources.LogSource) (Tailer, error) { return nil, errors.New("uhoh") }
	makeAPITailer := func(*sources.LogSource) (Tailer, error) { panic("api shouldn't be called") }

	source := &sources.LogSource{Messages: config.NewMessages()}
	tailer, err := (&factory{}).makeTailer(source, whichTailer, makeFileTailer, makeSocketTailer, makeAPITailer)
	require.NoError(t, err)
	require.NotNil(t, tailer)
	messages := source.Messages.GetMessages()
	require.NotNil(t, messages)

	require.NotNil(t, source.Messages.GetMessages())
	require.Contains(t, messages, "The socket tailer could not be made, falling back to file")
}

func TestMakeTailerSocketFallbackFailsToo(t *testing.T) {
	whichTailer := func(*sources.LogSource) whichTailer { return socket }
	makeFileTailer := func(*sources.LogSource) (Tailer, error) { return nil, errors.New("uhoh2") }
	makeSocketTailer := func(*sources.LogSource) (Tailer, error) { return nil, errors.New("uhoh1") }
	makeAPITailer := func(*sources.LogSource) (Tailer, error) { panic("api shouldn't be called") }
	source := &sources.LogSource{Messages: config.NewMessages()}

	tailer, err := (&factory{}).makeTailer(source, whichTailer, makeFileTailer, makeSocketTailer, makeAPITailer)
	require.ErrorContains(t, err, "uhoh2")
	require.Nil(t, tailer)
}

func TestMakeTailerAPI(t *testing.T) {
	whichTailer := func(*sources.LogSource) whichTailer { return api }
	makeFileTailer := func(*sources.LogSource) (Tailer, error) { panic("file shouldn't be called") }
	makeSocketTailer := func(*sources.LogSource) (Tailer, error) { panic("socket shouldn't be called") }
	makeAPITailer := func(*sources.LogSource) (Tailer, error) { return &TestTailer{}, nil }

	tailer, err := (&factory{}).makeTailer(&sources.LogSource{}, whichTailer, makeFileTailer, makeSocketTailer, makeAPITailer)
	require.NoError(t, err)
	require.NotNil(t, tailer)
}

func TestMakeTailerAPIFails(t *testing.T) {
	whichTailer := func(*sources.LogSource) whichTailer { return api }
	makeFileTailer := func(*sources.LogSource) (Tailer, error) { panic("file shouldn't be called") }
	makeSocketTailer := func(*sources.LogSource) (Tailer, error) { panic("socket shouldn't be called") }
	makeAPITailer := func(*sources.LogSource) (Tailer, error) { return nil, errors.New("uhoh") }

	source := &sources.LogSource{Messages: config.NewMessages()}
	tailer, err := (&factory{}).makeTailer(source, whichTailer, makeFileTailer, makeSocketTailer, makeAPITailer)
	messages := source.Messages.GetMessages()

	require.ErrorContains(t, err, "uhoh")
	require.Nil(t, tailer)
	require.NotNil(t, messages)
	require.Contains(t, messages, "The API tailer could not be made")
}
