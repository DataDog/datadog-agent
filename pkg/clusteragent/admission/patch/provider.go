// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package patch

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/telemetry"
	"github.com/DataDog/datadog-agent/pkg/config"
	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
)

type patchProvider interface {
	start(stopCh <-chan struct{})
	subscribe(kind TargetObjKind) chan Request
}

func newAPMProvider(rcClient *rcclient.Client, isLeaderNotif <-chan struct{}, telemetryCollector telemetry.TelemetryCollector, clusterName string, webhook *autoinstrumentation.Webhook) (patchProvider, error) {
	if config.Datadog.GetBool("remote_configuration.enabled") {
		return newRemoteConfigProvider(rcClient, isLeaderNotif, telemetryCollector, clusterName, webhook)
	}
	if config.Datadog.GetBool("admission_controller.apm.file_provider_path") {
		// Use the file config provider for e2e testing only (it replaces RC as a source of configs)
		file := config.Datadog.GetString("admission_controller.apm.file_provider_path")
		return newfileProvider(file, isLeaderNotif, clusterName), nil
	}
	return nil, errors.New("remote config is disabled")
}
