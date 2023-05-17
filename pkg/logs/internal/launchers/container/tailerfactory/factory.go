// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

// Package tailerfactory implements the logic required to determine which kind
// of tailer to use for a container-related LogSource, and to create that tailer.
package tailerfactory

import (
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/util/containersorpods"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	dockerutilPkg "github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

// Factory supports making new tailers.
type Factory interface {
	// MakeTailer creates a new tailer for the given LogSource.
	MakeTailer(source *sources.LogSource) (Tailer, error)
}

// factory encapsulates the information required to determine which kind
// of tailer to use for a container-related LogSource.
type factory struct {
	// sources allows adding new sources to the agent, for child file sources
	// (temporary)
	sources *sources.LogSources

	// pipelineProvider provides pipelines for the instantiated tailers
	pipelineProvider pipeline.Provider

	// registry is the auditor/registry, used both to look for existing offsets
	// and to create new tailers.
	registry auditor.Registry

	// workloadmetaStore is the global WLM store containing information about
	// containers and pods.
	workloadmetaStore workloadmeta.Store

	// cop allows the factory to determine whether the agent is logging
	// containers or pods.
	cop containersorpods.Chooser

	// dockerutil memoizes a DockerUtil instance; fetch this with getDockerUtil().
	dockerutil *dockerutilPkg.DockerUtil
}

var _ Factory = (*factory)(nil)

// New creates a new Factory.
func New(sources *sources.LogSources, pipelineProvider pipeline.Provider, registry auditor.Registry, workloadmetaStore workloadmeta.Store) Factory {
	return &factory{
		sources:           sources,
		pipelineProvider:  pipelineProvider,
		registry:          registry,
		workloadmetaStore: workloadmetaStore,
		cop:               containersorpods.NewChooser(),
	}
}

// MakeTailer implements Factory#MakeTailer.
func (tf *factory) MakeTailer(source *sources.LogSource) (Tailer, error) {
	return tf.makeTailer(source, tf.useFile, tf.makeFileTailer, tf.makeSocketTailer)
}

// makeTailer makes a new tailer, using function pointers to allow testing.
func (tf *factory) makeTailer(
	source *sources.LogSource,
	useFile func(*sources.LogSource) bool,
	makeFileTailer func(*sources.LogSource) (Tailer, error),
	makeSocketTailer func(*sources.LogSource) (Tailer, error),
) (Tailer, error) {

	// depending on the result of useFile, prefer either file logging or socket
	// logging, but fall back to the opposite.

	switch useFile(source) {
	case true:
		t, err := makeFileTailer(source)
		if err == nil {
			return t, nil
		}
		source.Messages.AddMessage("fileTailerError", "The log file tailer could not be made, falling back to socket")
		log.Warnf("Could not make file tailer for source %s (falling back to socket): %v", source.Name, err)
		return makeSocketTailer(source)

	case false:
		t, err := makeSocketTailer(source)
		if err == nil {
			return t, nil
		}
		source.Messages.AddMessage("socketTailerError", "The socket tailer could not be made, falling back to file")
		log.Warnf("Could not make socket tailer for source %s (falling back to file): %v", source.Name, err)
		return makeFileTailer(source)
	}
	return nil, nil // unreachable
}
