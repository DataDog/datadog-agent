// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package agentchecks

import (
	"encoding/json"
	"path"

	"github.com/DataDog/datadog-agent/pkg/collector/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/collector/runner"
	"github.com/DataDog/datadog-agent/pkg/metadata/common"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/cihub/seelog"
)

// GetPayload builds a payload of all the agentchecks metadata
func GetPayload() *Payload {
	seelog.Info("here!!")
	agentChecksPayload := AgentChecksPayload{}

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
			seelog.Warnf("Error formatting loader error from check %s: %v", check, err)
		}
		status := []interface{}{
			check, check, "initialization", "ERROR", string(jsonErrs),
		}
		agentChecksPayload.AgentChecks = append(agentChecksPayload.AgentChecks, status)
	}

	hostPayload := host.Payload{}
	x, found := util.Cache.Get(path.Join(util.AgentCachePrefix, "hostMeta"))
	if found {
		hostPayload = x.(host.Payload)
	}

	cp := common.GetPayload()
	payload := &Payload{
		CommonPayload{*cp},
		HostPayload{hostPayload},
		agentChecksPayload,
	}

	return payload
}
