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

type agentCheckResult struct {
	checkName  string
	configHash string
	status     string
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
func (c *collectorImpl) GetPayload(ctx context.Context) (*Payload, []agentCheckResult) {
	hostnameData, _ := c.hostname.Get(ctx)

	meta := hostMetadataUtils.GetMetaFromCache(ctx, c.config, c.hostname)
	meta.Hostname = hostnameData

	cp := hostMetadataUtils.GetCommonPayload(hostnameData, c.config)
	payload := &Payload{
		CommonPayload:    *cp,
		Meta:             *meta,
		ExternalhostTags: *externalhost.GetPayload(),
	}

	agentCheckResults := make([]agentCheckResult, 0)
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
			agentCheckResults = append(agentCheckResults, agentCheckResult{
				checkName:  s.CheckName,
				configHash: string(s.CheckID),
				status:     checkStatus,
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
		agentCheckResults = append(agentCheckResults, agentCheckResult{
			checkName:  check,
			configHash: "initialization",
			status:     "ERROR",
		})
	}

	configErrors := autodiscoveryimpl.GetConfigErrors()
	for check, e := range configErrors {
		status := []interface{}{
			check, check, "initialization", "ERROR", e,
		}
		payload.AgentChecks = append(payload.AgentChecks, status)
		agentCheckResults = append(agentCheckResults, agentCheckResult{
			checkName:  check,
			configHash: "initialization",
			status:     "ERROR",
		})
	}

	jmxStartupError := jmxStatus.GetStartupError()
	if jmxStartupError.LastError != "" {
		status := []interface{}{
			"jmx", "jmx", "initialization", "ERROR", jmxStartupError.LastError,
		}
		payload.AgentChecks = append(payload.AgentChecks, status)
		agentCheckResults = append(agentCheckResults, agentCheckResult{
			checkName:  "jmx",
			configHash: "initialization",
			status:     "ERROR",
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
					agentCheckResults = append(agentCheckResults, agentCheckResult{
						checkName:  checkName,
						configHash: checkID,
						status:     checkStatus,
					})
				}
			}
		}
	}
	return payload, agentCheckResults
}

// isMonitoredCheck checks if a check name is in the monitored checks list
func isMonitoredCheck(checkName string) bool {
	for _, monitored := range monitoredChecks {
		if monitored == checkName {
			return true
		}
	}
	return false
}

// sendAgentCheckMetrics creates and sends metrics series for monitored agent checks
func (c *collectorImpl) sendAgentCheckMetrics(ctx context.Context, timestamp time.Time, agentCheckResults []agentCheckResult) error {
	metricSerializer, isSet := c.metricSerializer.Get()
	if !isSet {
		return fmt.Errorf("metric serializer not available")
	}

	// Get hostname for the metrics
	hostname, _ := c.hostname.Get(ctx)

	// Create metrics series for each monitored check
	var series metrics.Series
	ts := float64(timestamp.Unix())

	// Count status by type for summary metrics
	statusCounts := map[string]int{"OK": 0, "WARNING": 0, "ERROR": 0}
	monitoredCheckCount := 0

	for _, checkResult := range agentCheckResults {
		// Only send metrics for monitored checks
		if !isMonitoredCheck(checkResult.checkName) {
			continue
		}

		monitoredCheckCount++
		statusCounts[checkResult.status]++

		// Create a gauge metric for each check with a numeric value based on status
		var statusValue float64
		switch checkResult.status {
		case "OK":
			statusValue = 0
		case "WARNING":
			statusValue = 1
		case "ERROR":
			statusValue = 2
		}

		// Create tags for the check
		tags := []string{
			fmt.Sprintf("check_name:%s", checkResult.checkName),
			fmt.Sprintf("config_hash:%s", checkResult.configHash),
			fmt.Sprintf("status:%s", checkResult.status),
		}

		// Create individual check status metric
		checkSerie := &metrics.Serie{
			Name:           "datadog.agent.check.status",
			Points:         []metrics.Point{{Value: statusValue, Ts: ts}},
			Tags:           tagset.CompositeTagsFromSlice(tags),
			Host:           hostname,
			MType:          metrics.APIGaugeType,
			SourceTypeName: "System",
		}
		series = append(series, checkSerie)
	}

	// Create summary metrics for each status type
	for status, count := range statusCounts {
		tags := []string{fmt.Sprintf("status:%s", status)}
		summarySerie := &metrics.Serie{
			Name:           "datadog.agent.check.count",
			Points:         []metrics.Point{{Value: float64(count), Ts: ts}},
			Tags:           tagset.CompositeTagsFromSlice(tags),
			Host:           hostname,
			MType:          metrics.APIGaugeType,
			SourceTypeName: "System",
		}
		series = append(series, summarySerie)
	}

	// Send the metrics series
	if len(series) > 0 {
		// Create an iterable series source
		iterableSeries := metrics.NewIterableSeries(func(se *metrics.Serie) {
			// This callback is called for each series during serialization
			log.Debugf("Sending agent check metric: %s = %v", se.Name, se.Points[0].Value)
		}, len(series), len(series))

		// Add all series to the iterable source
		for _, serie := range series {
			iterableSeries.Append(serie)
		}

		// Send the series
		if err := metricSerializer.SendIterableSeries(iterableSeries); err != nil {
			return fmt.Errorf("failed to send agent check metrics: %w", err)
		}

		log.Debugf("Sent %d agent check metrics for %d monitored checks", len(series), monitoredCheckCount)
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

	payload, agentCheckResults := c.GetPayload(ctx)
	if err := metricSerializer.SendAgentchecksMetadata(payload); err != nil {
		c.log.Errorf("unable to submit agentchecks metadata payload, %s", err)
	}

	// Send agent check metrics for monitored checks
	if err := c.sendAgentCheckMetrics(ctx, time.Now(), agentCheckResults); err != nil {
		c.log.Errorf("unable to send agent check metrics: %s", err)
	}

	return defaultInterval
}

var monitoredChecks = []string{
	"aerospike",
	"arangodb",
	"cassandra",
	"clickhouse",
	"cockroachdb",
	"couch",
	"couchbase",
	"druid",
	"elastic",
	"etcd",
	"foundationdb",
	"hazelcast",
	"ibm_db2",
	"ignite",
	"impala",
	"istio",
	"kyototycoon",
	"mapr",
	"marklogic",
	"mongo",
	"mysql",
	"openldap",
	"oracle",
	"postgres",
	"presto",
	"proxysql",
	"redisdb",
	"rethinkdb",
	"riak",
	"riakcs",
	"sap_hana",
	"scylla",
	"singlestore",
	"snowflake",
	"solr",
	"sqlserver",
	"teradata",
	"tokumx",
	"vertica",
	"voltdb",
	"apache",
	"ibm_was",
	"iis",
	"jboss_wildfly",
	"lighttpd",
	"nginx",
	"tomcat",
	"traffic_server",
	"weblogic",
	"envoy",
	"zk",
}
