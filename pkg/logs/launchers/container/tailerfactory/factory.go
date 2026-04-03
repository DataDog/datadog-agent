// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet || docker

// Package tailerfactory implements the logic required to determine which kind
// of tailer to use for a container-related LogSource, and to create that tailer.
package tailerfactory

import (
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/util/containersorpods"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers/container"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Factory supports making new tailers.
type Factory interface {
	// MakeTailer creates a new tailer for the given LogSource.
	MakeTailer(source *sources.LogSource) (Tailer, error)
}

type dockerUtilGetter interface {
	get() (container.DockerContainerLogInterface, error)
}

type dockerUtilGetterImpl struct {
	//nolint:unused
	cli container.DockerContainerLogInterface // this can trigger a false positive if only linted with kubelet tag
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
	workloadmetaStore option.Option[workloadmeta.Component]

	// cop allows the factory to determine whether the agent is logging
	// containers or pods.
	cop containersorpods.Chooser

	// dockerUtilGetter memoizes a DockerUtil instance; fetch this with dockerUtilGetter.get()
	dockerUtilGetter dockerUtilGetter

	tagger tagger.Component
}

var _ Factory = (*factory)(nil)

// New creates a new Factory.
func New(sources *sources.LogSources, pipelineProvider pipeline.Provider, registry auditor.Registry, workloadmetaStore option.Option[workloadmeta.Component], tagger tagger.Component) Factory {
	return &factory{
		sources:           sources,
		pipelineProvider:  pipelineProvider,
		registry:          registry,
		workloadmetaStore: workloadmetaStore,
		cop:               containersorpods.NewChooser(),
		dockerUtilGetter:  &dockerUtilGetterImpl{},
		tagger:            tagger,
	}
}

// MakeTailer implements Factory#MakeTailer.
func (tf *factory) MakeTailer(source *sources.LogSource) (Tailer, error) {
	return tf.makeTailer(source, tf.whichTailer, tf.makeFileTailer, tf.makeSocketTailer, tf.makeAPITailer)
}

// makeTailer makes a new tailer, using function pointers to allow testing.
func (tf *factory) makeTailer(
	source *sources.LogSource,
	whichTailer func(*sources.LogSource) whichTailer,
	makeFileTailer func(*sources.LogSource) (Tailer, error),
	makeSocketTailer func(*sources.LogSource) (Tailer, error),
	makeAPITailer func(*sources.LogSource) (Tailer, error),
) (Tailer, error) {

	switch whichTailer(source) {
	case api:
		t, err := makeAPITailer(source)
		if err != nil {
			source.Messages.AddMessage("APITailerError", "The API tailer could not be made")
			log.Warnf("Could not make API tailer for source %s: %v", source.Name, err)
			return nil, err
		}
		return t, nil
	// depending on the result of useFile, prefer either file logging or socket
	// logging, but fall back to the opposite.
	case file:
		t, err := makeFileTailer(source)
		if err == nil {
			return t, nil
		}
		source.Messages.AddMessage("fileTailerError", "The log file tailer could not be made, falling back to socket")
		log.Warnf("Could not make file tailer for source %s (falling back to socket): %v", source.Name, err)
		return makeSocketTailer(source)
	case socket:
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
