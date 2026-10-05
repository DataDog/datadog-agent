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

	flushValue := strings.TrimSpace(cfg.GetString("anomaly_detection.recording.flush_interval"))
	flushInterval := 60 * time.Second
	if flushValue != "" {
		parsed, err := time.ParseDuration(flushValue)
		if err != nil {
			return recorder.WriterConfig{}, fmt.Errorf("anomaly_detection.recording.flush_interval must be a valid duration: %w", err)
		}
		if parsed < 0 {
			return recorder.WriterConfig{}, fmt.Errorf("anomaly_detection.recording.flush_interval must be nonnegative: %s", flushValue)
		}
		if parsed > 0 {
			flushInterval = parsed
		}
	}

	retention := cfg.GetDuration("anomaly_detection.recording.retention")
	if retention <= 0 {
		retention = 24 * time.Hour
	}

	return recorder.WriterConfig{
		OutputDir: outputDir, FlushInterval: flushInterval, Retention: retention,
	}, nil
}
