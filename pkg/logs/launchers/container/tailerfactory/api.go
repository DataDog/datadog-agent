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
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/util/containersorpods"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers/container/tailerfactory/tailers"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// makeSocketTailer makes a socket-based tailer for the given source, or returns
// an error if it cannot do so (e.g., due to permission errors)
func (tf *factory) makeApiTailer(source *sources.LogSource) (Tailer, error) {
	containerID := source.Config.Identifier

	wmeta, ok := tf.workloadmetaStore.Get()
	if !ok {
		return nil, errors.New("workloadmeta store is not initialized")
	}
	pod, err := wmeta.GetKubernetesPodForContainer(containerID)
	if err != nil {
		return nil, fmt.Errorf("cannot find pod for container %q: %w", containerID, err)
	}

	var container *workloadmeta.OrchestratorContainer
	for _, pc := range pod.GetAllContainers() {
		if pc.ID == containerID {
			container = &pc
			break
		}
	}

	if container == nil {
		// this failure is impossible, as GetKubernetesPodForContainer found
		// the pod by searching for this container
		return nil, fmt.Errorf("cannot find container %q in pod %q", containerID, pod.Name)
	}

	ku, err := kubelet.GetKubeUtil()
	if err != nil {
		return nil, fmt.Errorf("Could not use kubelet client to collect logs for container %s: %w",
			containerID, err)
	}

	// Note that it's not clear from k8s documentation that the container logs,
	// or even the directory containing these logs, must exist at this point.
	// To avoid incorrectly falling back to socket logging (or failing to log
	// entirely) we do not check for the file here. This matches older
	// kubernetes-launcher behavior.

	//sourceName, serviceName := tf.defaultSourceAndService(source, containersorpods.LogPods)
	//// New file source that inherits most of its parent's properties
	//fileSource := sources.NewLogSource(
	//	fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, container.Name),
	//	&config.LogsConfig{
	//		Type:                        config.FileType,
	//		TailingMode:                 source.Config.TailingMode,
	//		Identifier:                  containerID,
	//		Path:                        path,
	//		Service:                     serviceName,
	//		Source:                      sourceName,
	//		Tags:                        source.Config.Tags,
	//		ProcessingRules:             source.Config.ProcessingRules,
	//		AutoMultiLine:               source.Config.AutoMultiLine,
	//		AutoMultiLineSampleSize:     source.Config.AutoMultiLineSampleSize,
	//		AutoMultiLineMatchThreshold: source.Config.AutoMultiLineMatchThreshold,
	//	})

	pipeline := tf.pipelineProvider.NextPipelineChan()
	readTimeout := time.Duration(pkgconfigsetup.Datadog().GetInt("logs_config.docker_client_read_timeout")) * time.Second

	// apply defaults for source and service directly to the LogSource struct (!!)
	source.Config.Source, source.Config.Service = tf.defaultSourceAndService(source, containersorpods.LogPods)

	return tailers.NewApiTailer(
		ku,
		containerID,
		container.Name,
		pod.Name,
		pod.Namespace,
		source,
		pipeline,
		readTimeout,
		tf.registry,
		tf.tagger,
	), nil
}
