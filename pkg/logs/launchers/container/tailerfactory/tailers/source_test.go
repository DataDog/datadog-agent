// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet || docker

package tailers

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// Start and Stop add and remove the source before returning, so a Stop right
// after a Start cannot leave the source behind.
func TestWrappedSourceStartStop(t *testing.T) {
	logSources := sources.NewLogSources()
	ws := &WrappedSource{
		Source:  sources.NewLogSource("src", &config.LogsConfig{Type: config.FileType, Path: "/tmp/x.log"}),
		Sources: logSources,
	}

	require.NoError(t, ws.Start())
	require.Equal(t, []*sources.LogSource{ws.Source}, logSources.GetSources())

	ws.Stop()
	require.Empty(t, logSources.GetSources())

	require.NoError(t, ws.Start())
	require.Equal(t, []*sources.LogSource{ws.Source}, logSources.GetSources())
}
