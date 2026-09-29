// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ibm_mq

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"
)

// The Agent check config is loaded from an embedded template (not an inline string
// constant) and expanded to one ibm_mq instance per queue manager, because the
// instance count is a scenario parameter (queue-manager count sweep). All instance option
// keys are verbatim from integrations-core ibm_mq/datadog_checks/ibm_mq/config.py.
//
//go:embed config/ibm_mq.yaml.tmpl
var checkConfigTemplate string

//go:embed config/datadog.yaml.tmpl
var agentConfigTemplate string

// checkInstance is one queue-manager instance in the rendered ibm_mq.d/conf.yaml.
type checkInstance struct {
	QueueManager string
	Port         int
}

// checkConfigData is the template context for the check config.
type checkConfigData struct {
	Channel               string
	Instances             []checkInstance
	AutoDiscoverQueues    bool
	QueueRegex            string
	ExplicitQueues        []string
	CollectResetQueue     bool
	MinCollectionInterval int
	MetricPatternsExclude []string
}

// renderCheckConfig expands the embedded template into an ibm_mq.d/conf.yaml with
// one instance per queue manager (localhost + the manager's listener port).
func renderCheckConfig(params *Params) (string, error) {
	data := checkConfigData{
		Channel:               params.Channel,
		AutoDiscoverQueues:    params.AutoDiscoverQueues,
		QueueRegex:            params.QueueRegex,
		ExplicitQueues:        params.ExplicitQueues,
		CollectResetQueue:     params.CollectResetQueue,
		MinCollectionInterval: params.MinCollectionInterval,
		MetricPatternsExclude: params.MetricPatternsExclude,
	}
	for i := 0; i < params.NQmgrs; i++ {
		data.Instances = append(data.Instances, checkInstance{
			QueueManager: fmt.Sprintf("%s%d", params.QMPrefix, i+1),
			Port:         params.BasePort + i,
		})
	}

	tmpl, err := template.New("ibm_mq").Parse(checkConfigTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing ibm_mq check config template: %w", err)
	}
	var out strings.Builder
	if err := tmpl.Execute(&out, data); err != nil {
		return "", fmt.Errorf("rendering ibm_mq check config: %w", err)
	}
	return out.String(), nil
}

// renderAgentConfig expands the datadog.yaml overlay used only when profiling is
// enabled (Python integration profiling and/or Go internal profiling).
func renderAgentConfig(params *Params) (string, error) {
	tmpl, err := template.New("datadog").Parse(agentConfigTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing datadog.yaml template: %w", err)
	}
	var out strings.Builder
	if err := tmpl.Execute(&out, params); err != nil {
		return "", fmt.Errorf("rendering datadog.yaml: %w", err)
	}
	return out.String(), nil
}
