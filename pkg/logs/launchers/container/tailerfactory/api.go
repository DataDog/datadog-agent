// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package tailerfactory

// This file handles creating API tailers which access logs by querying the Kubelet's API

import (
	"errors"
	"fmt"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/util/containersorpods"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers/container/tailerfactory/tailers"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

type kubeUtilGetter func() (kubelet.KubeUtilInterface, error)

var kubeUtilGet kubeUtilGetter = kubelet.GetKubeUtil

// makeAPITailer makes an API based tailer for the given source, or returns
// an error if it cannot do so (e.g., due to permission errors)
func (tf *factory) makeAPITailer(source *sources.LogSource) (Tailer, error) {
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

	ku, err := kubeUtilGet()
	if err != nil {
		return nil, fmt.Errorf("Could not use kubelet client to collect logs for container %s: %w",
			containerID, err)
	}

	// Note that it's not clear from k8s documentation that the container logs,
	// or even the directory containing these logs, must exist at this point.
	// To avoid incorrectly falling back to socket logging (or failing to log
	// entirely) we do not check for the file here. This matches older
	// kubernetes-launcher behavior.

	pipeline := tf.pipelineProvider.NextPipelineChan()
	readTimeout := pkgconfigsetup.Datadog().GetDuration("logs_config.kubelet_api_client_read_timeout")

	source.Config.Source, source.Config.Service = tf.defaultSourceAndService(source, containersorpods.LogPods)

	return tailers.NewAPITailer(
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
