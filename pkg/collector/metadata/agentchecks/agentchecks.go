// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package agentchecks

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/collector/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/collector/runner"
	"github.com/DataDog/datadog-agent/pkg/metadata/common"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/util/cache"

	log "github.com/cihub/seelog"
)

// GetPayload builds a payload of all the agentchecks metadata
func GetPayload() *Payload {
	agentChecksPayload := ACPayload{}

	// Grab the hostname from the cache
	var hostname string
	x, found := cache.Cache.Get(cache.BuildAgentKey("hostname"))
	if found {
		hostname = x.(string)
	}

	checkStats := runner.GetCheckStats()

	for _, stats := range checkStats {
		var status []interface{}
		if stats.LastError != "" {
			status = []interface{}{
				stats.CheckName, stats.CheckName, stats.CheckID, "ERROR", stats.LastError, "",
			}
		} else if len(stats.LastWarnings) != 0 {
			status = []interface{}{
				stats.CheckName, stats.CheckName, stats.CheckID, "WARNING", stats.LastWarnings, "",
			}
		} else {
			status = []interface{}{
				stats.CheckName, stats.CheckName, stats.CheckID, "OK", "", "",
			}
		}
		if status != nil {
			agentChecksPayload.AgentChecks = append(agentChecksPayload.AgentChecks, status)
		}
	}

	loaderErrors := autodiscovery.GetLoaderErrors()

	for check, errs := range loaderErrors {
		jsonErrs, err := json.Marshal(errs)
		if err != nil {
			log.Warnf("Error formatting loader error from check %s: %v", check, err)
		}
		status := []interface{}{
			check, check, "initialization", "ERROR", string(jsonErrs),
		}
		agentChecksPayload.AgentChecks = append(agentChecksPayload.AgentChecks, status)
	}

	// Grab the non agent checks information
	metaPayload := host.GetMeta()
	metaPayload.Hostname = hostname
	cp := common.GetPayload(hostname)
	payload := &Payload{
		CommonPayload{*cp},
		MetaPayload{*metaPayload},
		agentChecksPayload,
	}

	return payload
}
