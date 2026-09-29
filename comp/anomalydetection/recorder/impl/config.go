// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build anomalydetection_recorder

package recorderimpl

import (
	"fmt"
	"strings"
	"time"

	recorder "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
)

func writerConfig(cfg config.Component) (recorder.WriterConfig, error) {
	outputDir := cfg.GetString("anomaly_detection.recording.output_dir")
	if strings.TrimSpace(outputDir) == "" {
		return recorder.WriterConfig{}, fmt.Errorf("anomaly_detection.recording.output_dir not set")
	}

	flushSeconds := int64(cfg.GetInt("anomaly_detection.recording.flush_interval"))
	if flushSeconds < 0 || flushSeconds > (1<<63-1)/int64(time.Second) {
		return recorder.WriterConfig{}, fmt.Errorf("anomaly_detection.recording.flush_interval must be a nonnegative number of seconds within time.Duration range: %d", flushSeconds)
	}
	flushInterval := time.Duration(flushSeconds) * time.Second
	if flushSeconds == 0 {
		flushInterval = 60 * time.Second
	}

	retention := cfg.GetDuration("anomaly_detection.recording.retention")
	if retention <= 0 {
		retention = 24 * time.Hour
	}

	return recorder.WriterConfig{
		OutputDir: outputDir, FlushInterval: flushInterval, Retention: retention,
	}, nil
}
