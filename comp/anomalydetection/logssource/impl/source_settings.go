// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logssourceimpl

import config "github.com/DataDog/datadog-agent/comp/core/config"

const (
	anomalyDetectionLogsEnabledKey           = "anomaly_detection.logs.enabled"
	anomalyDetectionLogsContainersEnabledKey = "anomaly_detection.logs.containers.enabled"
	anomalyDetectionLogsKubeletEnabledKey    = "anomaly_detection.logs.kubelet.enabled"
)

type logSourceSettings struct {
	logsEnabled             bool
	containerSourcesEnabled bool
	kubeletSourceEnabled    bool
}

func newLogSourceSettings(cfg config.Component) logSourceSettings {
	return logSourceSettings{
		logsEnabled:             configBoolDefaultTrue(cfg, anomalyDetectionLogsEnabledKey),
		containerSourcesEnabled: configBoolDefaultTrue(cfg, anomalyDetectionLogsContainersEnabledKey),
		kubeletSourceEnabled:    configBoolDefaultTrue(cfg, anomalyDetectionLogsKubeletEnabledKey),
	}
}

func configBoolDefaultTrue(cfg config.Component, key string) bool {
	return !cfg.IsConfigured(key) || cfg.GetBool(key)
}

func (s logSourceSettings) shouldStart(observerAvailable, workloadmetaAvailable, observerRequired, recordingEnabled bool) bool {
	if !observerAvailable {
		return false
	}
	if !recordingEnabled && (!observerRequired || !s.logsEnabled) {
		return false
	}
	return s.kubeletSourceEnabled || (s.containerSourcesEnabled && workloadmetaAvailable)
}
