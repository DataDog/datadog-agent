// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubelet

package tailers

func NewAPITailer(kubeutil kubelet.KubeUtilInterface, containerID, containerName, podName, podNamespace string, source *sources.LogSource, pipeline chan *message.Message, readTimeout time.Duration, registry auditor.Registry, tagger tagger.Component) *APITailer {
	return &APITailer{}
}

func (t *APITailer) Start() error {}
