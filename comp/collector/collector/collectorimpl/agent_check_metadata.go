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
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/externalhost"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	jmxStatus "github.com/DataDog/datadog-agent/pkg/status/jmx"
	"github.com/DataDog/datadog-agent/pkg/tagset"
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

type agentCheck struct {
	instanceType string
	instanceName string
	status       string
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

// GetPayload builds a payload of all the agentchecks metadata
func (c *collectorImpl) GetPayload(ctx context.Context) (*Payload, []agentCheck) {
	hostnameData, _ := c.hostname.Get(ctx)

	meta := hostMetadataUtils.GetMetaFromCache(ctx, c.config, c.hostname)
	meta.Hostname = hostnameData

	cp := hostMetadataUtils.GetCommonPayload(hostnameData, c.config)
	payload := &Payload{
		CommonPayload:    *cp,
		Meta:             *meta,
		ExternalhostTags: *externalhost.GetPayload(),
	}

	agentChecks := []agentCheck{}
	checkStats := expvars.GetCheckStats()
	for _, stats := range checkStats {
		for _, s := range stats {
			var status []interface{}
			var checkStatus string
			if s.LastError != "" {
				status = []interface{}{
					s.CheckName, s.CheckName, s.CheckID, "ERROR", s.LastError, "",
				}
				checkStatus = "ERROR"
			} else if len(s.LastWarnings) != 0 {
				status = []interface{}{
					s.CheckName, s.CheckName, s.CheckID, "WARNING", s.LastWarnings, "",
				}
				checkStatus = "WARNING"
			} else {
				status = []interface{}{
					s.CheckName, s.CheckName, s.CheckID, "OK", "", "",
				}
				checkStatus = "OK"
			}
			payload.AgentChecks = append(payload.AgentChecks, status)
			agentChecks = append(agentChecks, agentCheck{
				instanceType: s.CheckName,
				instanceName: string(s.CheckID),
				status:       checkStatus,
			})
		}
	}

	loaderErrors := collector.GetLoaderErrors()
	for check, errs := range loaderErrors {
		jsonErrs, err := json.Marshal(errs)
		if err != nil {
			log.Warnf("Error formatting loader error from check %s: %v", check, err)
		}
		status := []interface{}{
			check, check, "initialization", "ERROR", string(jsonErrs),
		}
		payload.AgentChecks = append(payload.AgentChecks, status)
		agentChecks = append(agentChecks, agentCheck{
			instanceType: "initialization",
			instanceName: check,
			status:       "ERROR",
		})
	}

	configErrors := autodiscoveryimpl.GetConfigErrors()
	for check, e := range configErrors {
		status := []interface{}{
			check, check, "initialization", "ERROR", e,
		}
		payload.AgentChecks = append(payload.AgentChecks, status)
		agentChecks = append(agentChecks, agentCheck{
			instanceType: "initialization",
			instanceName: check,
			status:       "ERROR",
		})
	}

	jmxStartupError := jmxStatus.GetStartupError()
	if jmxStartupError.LastError != "" {
		status := []interface{}{
			"jmx", "jmx", "initialization", "ERROR", jmxStartupError.LastError,
		}
		payload.AgentChecks = append(payload.AgentChecks, status)
		agentChecks = append(agentChecks, agentCheck{
			instanceType: "jmx",
			instanceName: "initialization",
			status:       "ERROR",
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
					status := []interface{}{
						checkName, checkName, checkID, checkStatus, checkError,
					}
					payload.AgentChecks = append(payload.AgentChecks, status)
					agentChecks = append(agentChecks, agentCheck{
						instanceType: checkName,
						instanceName: checkID,
						status:       checkStatus,
					})
				}
			}
		}
	}
	return payload, agentChecks
}

// sendAgentCheckMetrics creates and sends metrics series for agent checks
func (c *collectorImpl) sendAgentCheckMetricsFromPayload(ctx context.Context, timestamp time.Time, agentChecks []interface{}) error {
	// Get hostname for the metrics
	hostname, _ := c.hostname.Get(ctx)

	// Create metrics series for each monitored check
	ts := float64(timestamp.Unix())

	if len(agentChecks) == 0 {
		log.Debugf("No agent checks found in payload")
		return nil
	}

	for _, check := range agentChecks {
		check, ok := check.([]interface{})
		if !ok || len(check) < 4 {
			log.Warnf("Invalid check format in agent checks payload")
			continue
		}

		status := "unknown"
		if checkStatus, ok := check[3].(string); ok {
			switch checkStatus {
			case "OK":
				status = "healthy"
			case "WARNING":
				status = "warning"
			case "ERROR":
				status = "broken"
			}
		} else {
			log.Warnf("Invalid check status in agent checks payload")
		}

		// Create tags for the check
		tags := []string{
			fmt.Sprintf("integration_type:%v", check[1]),
			fmt.Sprintf("integration_name:%v", check[2]),
			fmt.Sprintf("status:%s", status),
		}

		log.Debugf("Sending agent check metric: %s = %+v", "datadog.agent.integration.status", tags)

		// Create individual check status metric
		aggregator.AddRecurrentSeries(&metrics.Serie{
			Name:   "datadog.agent.integration.status",
			Points: []metrics.Point{{Value: 1.0, Ts: ts}},
			Tags:   tagset.CompositeTagsFromSlice(tags),
			Host:   hostname,
			MType:  metrics.APIGaugeType,
		})
	}

	return nil
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

	payload, _ := c.GetPayload(ctx)
	if err := metricSerializer.SendAgentchecksMetadata(payload); err != nil {
		c.log.Errorf("unable to submit agentchecks metadata payload, %s", err)
	}

	// Send agent check metrics for monitored checks
	if err := c.sendAgentCheckMetricsFromPayload(ctx, time.Now(), payload.AgentChecks); err != nil {
		c.log.Errorf("unable to send agent check metrics: %s", err)
	}

	return defaultInterval
}
