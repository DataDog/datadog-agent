// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package clusteragent implements the status of the cluster agent
package clusteragent

import (
	"context"
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/externalmetrics"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/config"
	logsStatus "github.com/DataDog/datadog-agent/pkg/logs/status"
	"github.com/DataDog/datadog-agent/pkg/status/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/status/common"
	"github.com/DataDog/datadog-agent/pkg/status/endpoints"
	"github.com/DataDog/datadog-agent/pkg/status/render"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetStatus grabs the status from expvar and puts it into a map
func GetStatus(verbose bool) (map[string]interface{}, error) {
	// inventory is not enabled for the clusteragent/DCA so we pass nil to GetStatus
	stats, err := common.GetStatus(nil)
	if err != nil {
		return nil, err
	}

	stats["config"] = getDCAPartialConfig()
	stats["leaderelection"] = getLeaderElectionDetails()

	if config.Datadog.GetBool("compliance_config.enabled") {
		stats["logsStats"] = logsStatus.Get(verbose)
	}

	endpoints.PopulateStatus(stats)

	apiCl, apiErr := apiserver.GetAPIClient()
	if apiErr != nil {
		stats["custommetrics"] = map[string]string{"Error": apiErr.Error()}
		stats["admissionWebhook"] = map[string]string{"Error": apiErr.Error()}
	} else {
		stats["custommetrics"] = custommetrics.GetStatus(apiCl.Cl)
		stats["admissionWebhook"] = admission.GetStatus(apiCl.Cl)
	}

	if config.Datadog.GetBool("external_metrics_provider.use_datadogmetric_crd") {
		stats["externalmetrics"] = externalmetrics.GetStatus()
	} else {
		stats["externalmetrics"] = apiserver.GetStatus()
	}

	if config.Datadog.GetBool("cluster_checks.enabled") {
		cchecks, err := clusterchecks.GetStats()
		if err != nil {
			log.Errorf("Error grabbing clusterchecks stats: %s", err)
		} else {
			stats["clusterchecks"] = cchecks
		}
	}

	autodiscovery.PopulateStatus(stats)

	if config.Datadog.GetBool("orchestrator_explorer.enabled") {
		if apiErr != nil {
			stats["orchestrator"] = map[string]string{"Error": apiErr.Error()}
		} else {
			orchestratorStats := orchestrator.GetStatus(context.TODO(), apiCl.Cl)
			stats["orchestrator"] = orchestratorStats
		}
	}

	return stats, nil
}

// getDCAPartialConfig returns config parameters of interest for the status page.
func getDCAPartialConfig() map[string]string {
	conf := make(map[string]string)
	conf["log_level"] = config.Datadog.GetString("log_level")
	conf["confd_path"] = config.Datadog.GetString("confd_path")
	return conf
}

// GetAndFormatStatus gets and formats the status all in one go
func GetAndFormatStatus() ([]byte, error) {
	s, err := GetStatus(true)
	if err != nil {
		log.Infof("Error while getting status %q", err)
		return nil, err
	}
	statusJSON, err := json.Marshal(s)
	if err != nil {
		log.Infof("Error while marshalling %q", err)
		return nil, err
	}
	st, err := render.FormatDCAStatus(statusJSON)
	if err != nil {
		log.Infof("Error formatting the status %q", err)
		return nil, err
	}
	return []byte(st), nil
}
