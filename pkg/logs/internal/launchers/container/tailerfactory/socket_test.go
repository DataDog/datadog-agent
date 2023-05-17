// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package tailerfactory

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers/container/tailerfactory/tailers"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/util/containersorpods"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	dockerutilPkg "github.com/DataDog/datadog-agent/pkg/util/docker"
)

func TestMakeSocketTailer_not_docker(t *testing.T) {
	tf := &factory{}
	source := sources.NewLogSource("test", &config.LogsConfig{
		Type: "notdocker",
	})
	_, err := tf.makeSocketTailer(source)
	require.ErrorContains(t, err, "only supported for docker")
}

func TestMakeSocketTailer_success(t *testing.T) {
	dockerutilPkg.EnableTestingMode()

	tf := &factory{
		pipelineProvider: pipeline.NewMockProvider(),
		cop:              containersorpods.NewDecidedChooser(containersorpods.LogContainers),
	}
	source := sources.NewLogSource("test", &config.LogsConfig{
		Type:       "docker",
		Identifier: "abc",
		Source:     "src",
		Service:    "svc",
	})
	dst, err := tf.makeSocketTailer(source)
	require.NoError(t, err)
	require.Equal(t, "abc", dst.(*tailers.DockerSocketTailer).ContainerID)
}
