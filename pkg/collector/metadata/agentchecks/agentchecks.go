// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package agentchecks

import (
	"path"

	"github.com/DataDog/datadog-agent/pkg/collector/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/collector/runner"
	"github.com/DataDog/datadog-agent/pkg/metadata/v5"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/cihub/seelog"
)

// GetPayload builds a payload of all the agentchecks metadata
func GetPayload() *Payload {
	seelog.Info("here!!")
	agentChecksPayload := &AgentChecksPayload{}

	checkStats := runner.GetCheckStats()

	for check, stats := range checkStats {
		var status []interface{}
		if stats.LastError != "" {
			status = []interface{}{
				check, "", stats.CheckID, stats.LastError, "ERROR", "",
			}
		} else if len(stats.LastWarnings) != 0 {
			status = []interface{}{
				check, "", stats.CheckID, stats.LastWarnings, "WARNING", "",
			}
		} else {
			status = []interface{}{
				check, "", stats.CheckID, stats.LastWarnings, "OK", "",
			}
		}
		agentChecksPayload.AgentChecks = append(agentChecksPayload.AgentChecks, status)
	}

	loaderErrors := autodiscovery.GetLoaderErrors()

	for check, errs := range loaderErrors {
		status := []interface{}{
			check, "", "initialization", "ERROR", errs,
		}
		agentChecksPayload.AgentChecks = append(agentChecksPayload.AgentChecks, status)
	}

	v5Payload := v5.Payload{}
	x, found := util.Cache.Get(path.Join(util.AgentCachePrefix, "metav5"))
	if found {
		v5Payload = x.(v5.Payload)
	}

	metaPayload := V5Payload{v5Payload}

	payload := &Payload{
		*agentChecksPayload,
		metaPayload,
	}

	seelog.Infof("payload: %v", payload)

	return payload
}
