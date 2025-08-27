// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package shellscript

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

type Metric struct {
	Type  string   `json:"type"`
	Name  string   `json:"name"`
	Value float64  `json:"value"`
	Tags  []string `json:"tags"`
}

type Payload struct {
	Metrics []Metric `json:"metrics"`
}

type shellScriptCheck struct {
	sendermanager  sender.SenderManager
	tagger         tagger.Component
	name           string
	scriptPath     string
	id             checkid.ID
	initConfig     integration.Data
	instanceConfig integration.Data
}

func newCheck(sendermanager sender.SenderManager, tagger tagger.Component, name string, scriptPath string) (check.Check, error) {
	return &shellScriptCheck{
		sendermanager: sendermanager,
		tagger:        tagger,
		name:          name,
		scriptPath:    scriptPath,
	}, nil
}

func (c *shellScriptCheck) Run() error {
	// Execute the shell script
	cmd := exec.Command(c.scriptPath)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("INIT_CONFIG=%s", c.InitConfig()),
		fmt.Sprintf("INSTANCE_CONFIG=%s", c.InstanceConfig()),
	)

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to execute script %s: %v", c.scriptPath, err)
	}

	// Parse JSON payload
	var payload Payload
	if err := json.Unmarshal(output, &payload); err != nil {
		return fmt.Errorf("failed to parse JSON output from script %s: %v", c.scriptPath, err)
	}

	// Process metrics and send them
	sender, err := c.sendermanager.GetSender(c.id)
	if err != nil {
		return fmt.Errorf("failed to get sender for shell script check %s: %v", c.name, err)
	}

	hostn, _ := hostname.Get(context.TODO())

	for _, metric := range payload.Metrics {
		tags := metric.Tags

		// Add global tags from the agent
		extraTags, err := c.tagger.GlobalTags(types.LowCardinality)
		if err != nil {
			fmt.Printf("failed to get global tags: %s\n", err.Error())
		} else {
			tags = append(tags, extraTags...)
		}

		fmt.Printf("Metric: %s = %f (type: %s, tags: %v)\n", metric.Name, metric.Value, metric.Type, tags)

		// Send metric based on type
		switch metric.Type {
		case "gauge":
			sender.Gauge(metric.Name, metric.Value, hostn, tags)
		case "rate":
			sender.Rate(metric.Name, metric.Value, hostn, tags)
		case "count":
			sender.Count(metric.Name, metric.Value, hostn, tags)
		case "monotonic_count":
			sender.MonotonicCountWithFlushFirstValue(metric.Name, metric.Value, hostn, tags, false)
		case "counter":
			sender.Counter(metric.Name, metric.Value, hostn, tags)
		case "histogram":
			sender.Histogram(metric.Name, metric.Value, hostn, tags)
		case "historate":
			sender.Historate(metric.Name, metric.Value, hostn, tags)
		default:
			fmt.Printf("Warning: unknown metric type %s for metric %s\n", metric.Type, metric.Name)
		}
	}

	return nil
}

func (c *shellScriptCheck) Stop() {}

func (c *shellScriptCheck) Cancel() {}

func (c *shellScriptCheck) String() string {
	return c.name
}

func (c *shellScriptCheck) Version() string {
	return ""
}

func (c *shellScriptCheck) IsTelemetryEnabled() bool {
	return false
}

func (c *shellScriptCheck) ConfigSource() string {
	return ""
}

func (*shellScriptCheck) Loader() string {
	return "shellscript"
}

func (c *shellScriptCheck) InitConfig() string {
	return string(c.initConfig)
}

func (c *shellScriptCheck) InstanceConfig() string {
	return string(c.instanceConfig)
}

func (c *shellScriptCheck) GetWarnings() []error {
	return []error{}
}

func (c *shellScriptCheck) Configure(_senderManager sender.SenderManager, integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, _source string) error {
	// Generate check ID
	c.id = checkid.BuildID(c.String(), integrationConfigDigest, data, initConfig)
	c.initConfig = initConfig
	c.instanceConfig = data

	return nil
}

func (c *shellScriptCheck) GetSenderStats() (stats.SenderStats, error) {
	return stats.SenderStats{}, nil
}

func (c *shellScriptCheck) Interval() time.Duration {
	return time.Hour
}

func (c *shellScriptCheck) ID() checkid.ID {
	return c.id
}

func (c *shellScriptCheck) GetDiagnoses() ([]diagnose.Diagnosis, error) {
	return []diagnose.Diagnosis{}, nil
}

func (c *shellScriptCheck) IsHASupported() bool {
	return false
}
