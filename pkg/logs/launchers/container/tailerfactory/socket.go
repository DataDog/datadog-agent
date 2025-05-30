// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package tailerfactory

// This file handles creating docker tailers which access the container runtime
// via socket.

import (
	"errors"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers/container"
	dockerutilPkg "github.com/DataDog/datadog-agent/pkg/util/docker"
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers/container/tailerfactory/tailers"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// get gets a DockerUtil instance, either returning a memoized value
// or trying to create a new one.
func (dug *dockerUtilGetterImpl) get() (container.DockerContainerLogInterface, error) {
	if dug.cli == nil {
		var err error
		dug.cli, err = dockerutilPkg.GetDockerUtil()
		if err != nil {
			return nil, err
		}
	}
	return dug.cli, nil
}

// makeSocketTailer makes a socket-based tailer for the given source, or returns
// an error if it cannot do so (e.g., due to permission errors)
func (tf *factory) makeSocketTailer(source *sources.LogSource) (Tailer, error) {
	containerID := source.Config.Identifier

	// this function may eventually support other sockets (such as podman) but for the
	// moment only supports docker.
	if source.Config.Type != "docker" {
		return nil, errors.New("socket tailing is only supported for docker")
	}

	du, err := tf.dockerUtilGetter.get()
	// if LogWhat == LogPods, this might fail because the docker socket is unavailable.  The
	// error will trigger a fallback to file-based logging.
	if err != nil {
		return nil, fmt.Errorf("Could not use docker client to collect logs for container %s: %w",
			containerID, err)
	}

	// Otherwise, if DockerUtil is available, then the docker socket was
	// available at some point, so chances are good that tailing will succeed.

	pipeline := tf.pipelineProvider.NextPipelineChan()
	readTimeout := time.Duration(pkgconfigsetup.Datadog().GetInt("logs_config.docker_client_read_timeout")) * time.Second

	// apply defaults for source and service directly to the LogSource struct (!!)
	source.Config.Source, source.Config.Service = tf.defaultSourceAndService(source, tf.cop.Get())

	return tailers.NewDockerSocketTailer(
		du,
		containerID,
		source,
		pipeline,
		readTimeout,
		tf.registry,
		tf.tagger,
	), nil
}
