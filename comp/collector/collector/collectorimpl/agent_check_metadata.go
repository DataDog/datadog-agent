// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package collectorimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	hostMetadataUtils "github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/utils"
	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/externalhost"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	jmxStatus "github.com/DataDog/datadog-agent/pkg/status/jmx"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

//
// The "agent_check" metadata payload contains information about all running checks and the additional hostnames they
// added to the Agent.
//

const (
	defaultInterval   = 10 * time.Minute
	firstPayloadDelay = 1 * time.Minute
)

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	hostMetadataUtils.CommonPayload
	Meta             hostMetadataUtils.Meta `json:"meta"`
	AgentChecks      []interface{}          `json:"agent_checks"`
	ExternalhostTags externalhost.Payload   `json:"external_host_tags"`
}

// MarshalJSON serialization a Payload to JSON
func (p *Payload) MarshalJSON() ([]byte, error) {
	// use an alias to avoid infinite recursion while serializing
	type PayloadAlias Payload

	return json.Marshal((*PayloadAlias)(p))
}

// SplitPayload breaks the payload into times number of pieces
func (p *Payload) SplitPayload(_ int) ([]marshaler.AbstractMarshaler, error) {
	return nil, fmt.Errorf("AgentChecks Payload splitting is not implemented")
}

func (c *collectorImpl) GetChecksResults() []map[string]interface{} {
	cr := make([]map[string]interface{}, 0)
	checkStats := expvars.GetCheckStats()
	for _, stats := range checkStats {
		for _, s := range stats {
			if s.LastError != "" {
				cr = append(cr, map[string]interface{}{
					"check_name": s.CheckName,
					"check_id":   s.CheckID,
					"status":     "ERROR",
					"error":      s.LastError,
				})
			} else if len(s.LastWarnings) != 0 {
				cr = append(cr, map[string]interface{}{
					"check_name": s.CheckName,
					"check_id":   s.CheckID,
					"status":     "WARNING",
					"error":      s.LastWarnings,
				})
			} else {
				cr = append(cr, map[string]interface{}{
					"check_name": s.CheckName,
					"check_id":   s.CheckID,
					"status":     "OK",
					"error":      "",
				})
			}

		}
	}

	loaderErrors := collector.GetLoaderErrors()
	for check, errs := range loaderErrors {
		jsonErrs, err := json.Marshal(errs)
		if err != nil {
			log.Warnf("Error formatting loader error from check %s: %v", check, err)
		}
		cr = append(cr, map[string]interface{}{
			"check_name": check,
			"check_id":   "initialization",
			"status":     "ERROR",
			"error":      string(jsonErrs),
		})
	}

	configErrors := autodiscoveryimpl.GetConfigErrors()
	for check, e := range configErrors {
		cr = append(cr, map[string]interface{}{
			"check_name": check,
			"check_id":   "initialization",
			"status":     "ERROR",
			"error":      e,
		})
	}

	jmxStartupError := jmxStatus.GetStartupError()
	if jmxStartupError.LastError != "" {
		cr = append(cr, map[string]interface{}{
			"check_name": "jmx",
			"check_id":   "initialization",
			"status":     "ERROR",
			"error":      jmxStartupError.LastError,
		})
	}

	stats := map[string]interface{}{}
	jmxStatus.PopulateStatus(stats)
	if _, ok := stats["JMXStatus"]; ok {
		if status, ok := stats["JMXStatus"].(jmxStatus.Status); ok {
			for checkName, checksRaw := range status.ChecksStatus.InitializedChecks {
				checks, ok := checksRaw.([]interface{})
				if !ok {
					continue
				}
				for _, checkRaw := range checks {
					check, ok := checkRaw.(map[string]interface{})
					// The default check status is OK, so if there is no status, it means the check is OK
					if !ok {
						continue
					}
					checkStatus, ok := check["status"].(string)
					if !ok {
						checkStatus = "OK"
					}
					checkID, ok := check["instance_name"].(string)
					if !ok {
						checkID = checkName
					} else {
						checkID = fmt.Sprintf("%s:%s", checkName, checkID)
					}
					checkError, ok := check["message"].(string)
					if !ok {
						checkError = ""
					}
					cr = append(cr, map[string]interface{}{
						"check_name": checkName,
						"check_id":   checkID,
						"status":     checkStatus,
						"error":      checkError,
					})
				}
			}
		}
	}

	return cr
}

// GetPayload builds a payload of all the agentchecks metadata
func (c *collectorImpl) GetPayload(ctx context.Context) *Payload {
	hostnameData, _ := c.hostname.Get(ctx)

	meta := hostMetadataUtils.GetMetaFromCache(ctx, c.config, c.hostname)
	meta.Hostname = hostnameData

	cp := hostMetadataUtils.GetCommonPayload(hostnameData, c.config)
	payload := &Payload{
		CommonPayload:    *cp,
		Meta:             *meta,
		ExternalhostTags: *externalhost.GetPayload(),
	}

	cr := c.GetChecksResults()
	for _, c := range cr {
		payload.AgentChecks = append(payload.AgentChecks, []interface{}{
			c["check_name"], c["check_name"], c["check_id"], c["status"], c["error"], "",
		})
	}

	return payload
}

func (c *collectorImpl) collectMetadata(ctx context.Context) time.Duration {
	metricSerializer, isSet := c.metricSerializer.Get()
	if !isSet {
		return defaultInterval
	}

	// We want to wait 1 min before collecting and sending the first payload.
	if time.Since(c.createdAt) < firstPayloadDelay {
		return firstPayloadDelay - time.Since(c.createdAt)
	}

	payload := c.GetPayload(ctx)
	if err := metricSerializer.SendAgentchecksMetadata(payload); err != nil {
		c.log.Errorf("unable to submit agentchecks metadata payload, %s", err)
	}
	return defaultInterval
}
