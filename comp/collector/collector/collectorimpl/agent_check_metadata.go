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
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
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

// GetPayload builds a payload of all the agentchecks metadata
func (c *collectorImpl) GetPayload(ctx context.Context) *Payload {
	hostnameData, _ := hostname.Get(ctx)

	meta := hostMetadataUtils.GetMetaFromCache(ctx, c.config)
	meta.Hostname = hostnameData

	cp := hostMetadataUtils.GetCommonPayload(hostnameData, c.config)
	payload := &Payload{
		CommonPayload:    *cp,
		Meta:             *meta,
		ExternalhostTags: *externalhost.GetPayload(),
	}

	checkStats := expvars.GetCheckStats()
	for _, stats := range checkStats {
		for _, s := range stats {
			var status []interface{}
			if s.LastError != "" {
				status = []interface{}{
					s.CheckName, s.CheckName, s.CheckID, "ERROR", s.LastError, "",
				}
			} else if len(s.LastWarnings) != 0 {
				status = []interface{}{
					s.CheckName, s.CheckName, s.CheckID, "WARNING", s.LastWarnings, "",
				}
			} else {
				status = []interface{}{
					s.CheckName, s.CheckName, s.CheckID, "OK", "", "",
				}
			}
			payload.AgentChecks = append(payload.AgentChecks, status)
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
	}

	configErrors := autodiscoveryimpl.GetConfigErrors()
	for check, e := range configErrors {
		status := []interface{}{
			check, check, "initialization", "ERROR", e,
		}
		payload.AgentChecks = append(payload.AgentChecks, status)
	}

	jmxStartupError := jmxStatus.GetStartupError()
	if jmxStartupError.LastError != "" {
		status := []interface{}{
			"jmx", "jmx", "initialization", "ERROR", jmxStartupError.LastError,
		}
		payload.AgentChecks = append(payload.AgentChecks, status)
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
