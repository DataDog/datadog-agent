// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package collectorimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
	"unicode/utf8"

	autodiscoveryimpl "github.com/DataDog/datadog-agent/comp/core/autodiscovery/impl"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	hostMetadataUtils "github.com/DataDog/datadog-agent/comp/metadata/host/impl/utils"
	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/externalhost"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
	"github.com/DataDog/datadog-agent/pkg/jmxfetch"
	jmxStatus "github.com/DataDog/datadog-agent/pkg/status/jmx"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"go.yaml.in/yaml/v3"
)

//
// The "agent_check" metadata payload contains information about all running checks and the additional hostnames they
// added to the Agent.
//

const (
	defaultInterval   = 10 * time.Minute
	firstPayloadDelay = 1 * time.Minute

	// maxCheckErrorLength caps the size of a single check error or warning message in the payload, so that
	// large messages do not inflate the whole payload.
	maxCheckErrorLength = 4096

	truncatedMarkerFormat = "\n[... %d bytes truncated ...]\n"
)

// Kinds of message of the payload, used as tag of the oversized message telemetry.
const (
	kindError      = "error"
	kindWarning    = "warning"
	kindLoader     = "loader"
	kindConfig     = "config"
	kindJMXStartup = "jmx_startup"
)

// oversizedMetrics holds the telemetry of the check messages exceeding the size cap. Both metrics are part of the
// cross-org agent telemetry, see the "checks" profile in comp/core/agenttelemetry/impl/defaultProfiles.yaml.
//
// The "truncated" tag tells whether the message was actually truncated or only measured, so the oversized messages
// stay visible when the truncation is disabled.
type oversizedMetrics struct {
	messages telemetry.Counter
	bytes    telemetry.Histogram
}

func newOversizedMetrics(tel telemetry.Component) *oversizedMetrics {
	return &oversizedMetrics{
		messages: tel.NewCounter("checks", "metadata_message_oversized",
			[]string{"check_name", "kind", "truncated"},
			"Check messages sent in the agent_checks metadata payload that exceed the size cap"),
		bytes: tel.NewHistogram("checks", "metadata_message_oversized_bytes",
			[]string{"kind"},
			"Original size, in bytes, of the check messages exceeding the size cap",
			[]float64{8 << 10, 16 << 10, 64 << 10, 256 << 10, 1 << 20, 4 << 20, 16 << 20}),
	}
}

var defaultOversizedMetrics = newOversizedMetrics(telemetryimpl.GetCompatComponent())

// truncateCheckError limits msg to maxCheckErrorLength bytes, marker included. It keeps both the beginning and
// the end of the message, as the end often holds the most useful part (e.g. the exception of a stack trace),
// and never splits a UTF-8 character.
func truncateCheckError(msg string) string {
	if len(msg) <= maxCheckErrorLength {
		return msg
	}
	// The marker for the actual number of dropped bytes can't be longer than this one.
	budget := maxCheckErrorLength - len(fmt.Sprintf(truncatedMarkerFormat, len(msg)))
	headEnd := budget / 2
	tailStart := len(msg) - (budget - headEnd)
	for i := 0; i < utf8.UTFMax && headEnd > 0 && !utf8.RuneStart(msg[headEnd]); i++ {
		headEnd--
	}
	for i := 0; i < utf8.UTFMax && tailStart < len(msg) && !utf8.RuneStart(msg[tailStart]); i++ {
		tailStart++
	}
	return msg[:headEnd] + fmt.Sprintf(truncatedMarkerFormat, tailStart-headEnd) + msg[tailStart:]
}

// messageTruncator applies the size cap to the check messages of the agent_checks metadata payload.
type messageTruncator struct {
	enabled bool
	metrics *oversizedMetrics
}

// maybeTruncate returns msg, truncated if it exceeds maxCheckErrorLength and the truncation is enabled. An oversized
// message is recorded in the telemetry whether it is truncated or not, checkName and kind being its tags.
func (t messageTruncator) maybeTruncate(msg, checkName, kind string) string {
	if len(msg) <= maxCheckErrorLength {
		return msg
	}
	t.metrics.messages.Inc(checkName, kind, strconv.FormatBool(t.enabled))
	t.metrics.bytes.Observe(float64(len(msg)), kind)
	if !t.enabled {
		return msg
	}
	return truncateCheckError(msg)
}

// truncateWarnings truncates each warning of a check.
func (t messageTruncator) truncateWarnings(warnings []string, checkName string) []string {
	truncated := make([]string, len(warnings))
	for i, w := range warnings {
		truncated[i] = t.maybeTruncate(w, checkName, kindWarning)
	}
	return truncated
}

// truncateLoaderErrors truncates each error of a loader name -> error map.
func (t messageTruncator) truncateLoaderErrors(errs map[string]string, checkName string) map[string]string {
	truncated := make(map[string]string, len(errs))
	for loader, e := range errs {
		truncated[loader] = t.maybeTruncate(e, checkName, kindLoader)
	}
	return truncated
}

// jmxInstanceData stores metadata about a JMX instance for building consistent check IDs
type jmxInstanceData struct {
	host string
	port string
	tags interface{}
}

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

	truncator := messageTruncator{
		enabled: c.config.GetBool("agent_checks_metadata_message_truncation_enabled"),
		metrics: defaultOversizedMetrics,
	}

	checkStats := expvars.GetCheckStats()
	for _, stats := range checkStats {
		for _, s := range stats {
			integrationTags := []string{}
			if check, found := c.get(s.CheckID); found {
				var err error
				integrationTags, err = collectTags(check.InstanceConfig())
				if err != nil {
					log.Infof("Error collecting tags from check %s: %v", check, err)
				}
			}
			var status []interface{}
			if s.LastError != "" {
				status = []interface{}{
					s.CheckName, s.CheckName, s.CheckID, "ERROR", truncator.maybeTruncate(s.LastError, s.CheckName, kindError), "", integrationTags,
				}
			} else if len(s.LastWarnings) != 0 {
				status = []interface{}{
					s.CheckName, s.CheckName, s.CheckID, "WARNING", truncator.truncateWarnings(s.LastWarnings, s.CheckName), "", integrationTags,
				}
			} else {
				status = []interface{}{
					s.CheckName, s.CheckName, s.CheckID, "OK", "", "", integrationTags,
				}
			}
			payload.AgentChecks = append(payload.AgentChecks, status)
		}
	}

	loaderErrors := collector.GetLoaderErrors()
	for check, errs := range loaderErrors {
		// Truncate each error rather than the marshalled result, so the payload keeps a valid JSON document.
		jsonErrs, err := json.Marshal(truncator.truncateLoaderErrors(errs, check))
		if err != nil {
			log.Warnf("Error formatting loader error from check %s: %v", check, err)
		}
		status := []interface{}{
			check, check, "initialization", "ERROR", string(jsonErrs), []string{},
		}
		payload.AgentChecks = append(payload.AgentChecks, status)
	}

	configErrors := autodiscoveryimpl.GetConfigErrors()
	for check, e := range configErrors {
		status := []interface{}{
			check, check, "initialization", "ERROR", truncator.maybeTruncate(e, check, kindConfig), []string{},
		}
		payload.AgentChecks = append(payload.AgentChecks, status)
	}

	jmxStartupError := jmxStatus.GetStartupError()
	if jmxStartupError.LastError != "" {
		status := []interface{}{
			"jmx", "jmx", "initialization", "ERROR", truncator.maybeTruncate(jmxStartupError.LastError, "jmx", kindJMXStartup), []string{},
		}
		payload.AgentChecks = append(payload.AgentChecks, status)
	}

	stats := map[string]interface{}{}
	jmxStatus.PopulateStatus(stats)
	instanceConfByName := map[string]*jmxInstanceData{}
	for _, config := range jmxfetch.GetScheduledConfigs() {
		for _, instance := range config.Instances {
			instanceconfig := map[interface{}]interface{}{}
			err := yaml.Unmarshal(instance, &instanceconfig)
			if err != nil {
				log.Errorf("invalid instance section: %s", err)
				continue
			}

			// Instance name is required to map this data
			instanceName, hasName := instanceconfig["name"].(string)
			if !hasName {
				continue
			}

			// Extract host with default
			host := "unknown"
			if h, ok := instanceconfig["host"]; ok {
				host = fmt.Sprint(h)
			}

			// Extract port with default
			port := "unknown"
			if p, ok := instanceconfig["port"]; ok {
				port = fmt.Sprint(p)
			}

			// Extract tags
			var tags interface{} = []string{}
			if tagsNode, ok := instanceconfig["tags"]; ok {
				tags = tagsNode
			}

			instanceConfByName[instanceName] = &jmxInstanceData{
				host: host,
				port: port,
				tags: tags,
			}
		}
	}

	if _, ok := stats["JMXStatus"]; ok {
		if status, ok := stats["JMXStatus"].(jmxStatus.Status); ok {
			for checkName, checksRaw := range status.ChecksStatus.InitializedChecks {
				checks, ok := checksRaw.([]interface{})
				if !ok {
					continue
				}
				for _, checkRaw := range checks {
					var tags interface{}
					check, ok := checkRaw.(map[string]interface{})
					// The default check status is OK, so if there is no status, it means the check is OK
					if !ok {
						continue
					}
					checkStatus, ok := check["status"].(string)
					if !ok {
						checkStatus = "OK"
					}

					instanceName, ok := check["instance_name"].(string)
					var checkID string
					if !ok {
						// No instance name - use just the check name
						checkID = checkName
						tags = []string{}
					} else {
						// Instance name exists - look up full instance data
						if instanceData, found := instanceConfByName[instanceName]; found {
							// Build checkID in format: checkName-host-port:instanceName
							// This matches the format used in inventory metadata (integrations_jmx.go:77-79)
							checkID = fmt.Sprintf("%s-%s-%s:%s", checkName, instanceData.host, instanceData.port, instanceName)
							tags = instanceData.tags
						} else {
							// Instance not found in config - fall back with unknown host/port
							log.Debugf("JMX instance %s not found in scheduled configs, using unknown host/port", instanceName)
							checkID = fmt.Sprintf("%s-unknown-unknown:%s", checkName, instanceName)
							tags = []string{}
						}
					}

					checkError, ok := check["message"].(string)
					if !ok {
						checkError = ""
					}

					status := []interface{}{
						checkName, checkName, checkID, checkStatus, checkError, "", tags,
					}
					payload.AgentChecks = append(payload.AgentChecks, status)
				}
			}
		}
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

func collectTags(config string) ([]string, error) {
	if config == "" {
		return []string{}, nil
	}

	var instanceconfig map[interface{}]interface{}
	unmarshalErr := yaml.Unmarshal([]byte(config), &instanceconfig)
	if unmarshalErr != nil {
		return []string{}, unmarshalErr
	}

	if tagsNode, ok := instanceconfig["tags"]; ok {
		if tags, ok := tagsNode.(string); ok {
			return []string{tags}, nil
		}
		if tags, ok := tagsNode.([]interface{}); ok {
			out := make([]string, 0, len(tags))
			for _, tag := range tags {
				out = append(out, tag.(string))
			}
			return out, nil
		}
	}

	return []string{}, nil

}
