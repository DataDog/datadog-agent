// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"time"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func initAnomalyDetectionRecording(config pkgconfigmodel.Setup) {
	config.BindEnvAndSetDefault("anomaly_detection.recording.enabled", false)
	config.BindEnvAndSetDefault("anomaly_detection.recording.output_dir", "/var/run/datadog/anomaly_detection")
	config.BindEnvAndSetDefault("anomaly_detection.recording.flush_interval", 60*time.Second)
	config.BindEnvAndSetDefault("anomaly_detection.recording.retention", 24*time.Hour)
}
