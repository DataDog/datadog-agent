// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metricslogsimpl

import (
	"encoding/json"

	metricslogs "github.com/DataDog/datadog-agent/comp/core/metricslogs/def"
)

// LogMetrics writes a batch of metrics as one structured JSON line to the
// agent's local log, at Debug level by default. It is a safe no-op for an
// empty batch.
func (c *metricsLogsComponent) LogMetrics(ms []*metricslogs.Metric, opts ...metricslogs.Option) error {
	if len(ms) == 0 {
		return nil
	}

	var cfg metricslogs.LogConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	metricEntries := make([]map[string]any, 0, len(ms))
	for _, m := range ms {
		metricEntries = append(metricEntries, map[string]any{
			"metric": m.Name,
			"type":   string(m.Type),
			"value":  m.Value,
			"tags":   m.Tags,
		})
	}

	data, err := json.Marshal(map[string]any{
		"message": "agent metrics batch",
		"metrics": metricEntries,
	})
	if err != nil {
		return err
	}

	if cfg.Level == metricslogs.LevelTrace {
		c.log.Tracef("metricslogs: %s", data)
	} else {
		c.log.Debugf("metricslogs: %s", data)
	}
	return nil
}
