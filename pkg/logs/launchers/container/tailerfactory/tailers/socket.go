// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

//nolint:revive // TODO(AML) Fix revive linter
package tailers

import (
	"context"
	"time"

	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	containerTailerPkg "github.com/DataDog/datadog-agent/pkg/logs/tailers/container"
	containerutilPkg "github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
)

// NOTE: once the docker launcher is removed, the inner Docker tailer can be
// modified to suit the Tailer interface directly and to handle connection
// failures on its own, and this wrapper will no longer be necessary.

type DockerSocketTailer struct {
	dockerutil containerTailerPkg.DockerContainerLogInterface
	base
}

// NewDockerSocketTailer Creates a new docker socket tailer
func NewDockerSocketTailer(dockerutil containerTailerPkg.DockerContainerLogInterface, containerID string, source *sources.LogSource, pipeline chan *message.Message, readTimeout time.Duration, registry auditor.Registry, tagger tagger.Component) *DockerSocketTailer {
	return &DockerSocketTailer{
		dockerutil: dockerutil,
		base: base{
			ContainerID: containerID,
			source:      source,
			pipeline:    pipeline,
			readTimeout: readTimeout,
			registry:    registry,
			tagger:      tagger,
			ctx:         nil,
			cancel:      nil,
			stopped:     nil,
		},
	}
}

// tryStartTailer tries to start the inner tailer, returning an erroredContainerID channel if
// successful.
func (t *DockerSocketTailer) tryStartTailer() (*containerTailerPkg.Tailer, chan string, error) {
	erroredContainerID := make(chan string)
	inner := containerTailerPkg.NewDockerTailer(
		t.dockerutil,
		t.ContainerID,
		t.source,
		t.pipeline,
		erroredContainerID,
		t.readTimeout,
		t.tagger,
		t.registry,
	)
	since, err := since(t.registry, inner.Identifier())
	if err != nil {
		log.Warnf("Could not recover tailing from last committed offset %v: %v",
			containerutilPkg.ShortContainerID(t.ContainerID), err)
		// (the `since` value is still valid)
	}

	err = inner.Start(since)
	if err != nil {
		return nil, nil, err
	}
	return inner, erroredContainerID, nil
}

// Start implements Tailer#Start.
func (t *DockerSocketTailer) Start() error {
	t.ctx, t.cancel = context.WithCancel(context.Background())
	t.stopped = make(chan struct{})
	go t.run(t.tryStartTailer, t.base.stopTailer)
	return nil
}
