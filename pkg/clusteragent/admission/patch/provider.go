// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package patch

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/remote"
)

type patchProvider interface {
	start(stopCh <-chan struct{})
	subscribe(kind TargetObjKind) chan PatchRequest
}

func newPatchProvider(rcClient *remote.Client, isLeaderNotif <-chan struct{}, clusterName string) (patchProvider, error) {
	if config.Datadog.GetBool("remote_configuration.enabled") {
		return newRemoteConfigProvider(rcClient, isLeaderNotif, clusterName)
	}
	if config.Datadog.GetBool("admission_controller.auto_instrumentation.patcher.fallback_to_file_provider") {
		// Use the file config provider for e2e testing only (it replaces RC as a source of configs)
		file := config.Datadog.GetString("admission_controller.auto_instrumentation.patcher.file_provider_path")
		return newfileProvider(file, isLeaderNotif, clusterName), nil
	}
	return nil, errors.New("remote config is disabled")
}
