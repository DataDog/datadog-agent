// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetry

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/status/render"

	dto "github.com/prometheus/client_model/go"
	"go.uber.org/fx"
)

type agenttelemetry struct {
	cfgComp    config.Component
	logComp    log.Component
	telComp    telemetry.Component
	statusComp status.Component
	hostComp   host.Component

	enabled bool
	sender  sender
	runner  runner
	atelCfg *Config

	cancelCtx context.Context
	cancel    context.CancelFunc
}

// FX-in compatibility
type dependencies struct {
	fx.In

	Log       log.Component
	Config    config.Component
	Telemetry telemetry.Component
	Status    status.Component
	Host      host.Component

	Lifecycle fx.Lifecycle
}

// FX-out compatibility
type provides struct {
	fx.Out

	Comp Component
}

// Interfacing with runner.
type job struct {
	atel     *agenttelemetry
	profiles []*Profile
	schedule Schedule
}

func (j job) Run() {
	j.atel.run(j.profiles)
}

// Passing metrics to sender Interfacing with sender
type agentmetric struct {
	metricName       string
	filteredMetrics  []*dto.Metric
	promMetricFamily *dto.MetricFamily
}

func createAgentTelemetrySender(
	cfgComp config.Component,
	logComp log.Component,
	hostComp host.Component,
) (sender, error) {
	client := newSenderClientImpl(cfgComp)
	sender, err := newSenderImpl(cfgComp, logComp, hostComp, client)
	if err != nil {
		logComp.Errorf("Failed to create agent telemetry sender: %s", err.Error())
	}
	return sender, err
}

func createAgentTelemetryProvider(
	cfgComp config.Component,
	logComp log.Component,
	telComp telemetry.Component,
	statusComp status.Component,
	hostComp host.Component,
	sender sender,
	runner runner) *agenttelemetry {
	// Parse Agent Telemetry Configuration configuration
	atelCfg, err := parseConfig(cfgComp)
	if err != nil {
		logComp.Errorf("Failed to parse agent telemetry config: %s", err.Error())
		return &agenttelemetry{}
	}
	if !atelCfg.Enabled {
		logComp.Info("Agent telemetry is disabled")
		return &agenttelemetry{}
	}

	return &agenttelemetry{
		enabled:    true,
		cfgComp:    cfgComp,
		logComp:    logComp,
		telComp:    telComp,
		statusComp: statusComp,
		hostComp:   hostComp,
		sender:     sender,
		runner:     runner,
		atelCfg:    atelCfg,
	}
}

// Factory function to create a new agent telemetry provider
func newAgentTelemetryProvider(deps dependencies) provides {
	sender, err := createAgentTelemetrySender(deps.Config, deps.Log, deps.Host)
	if err != nil {
		return provides{Comp: &agenttelemetry{}}
	}

	runner := newRunnerImpl()

	// Wire up the agent telemetry provider (TODO: use FX for sender, client and runner?)
	atel := createAgentTelemetryProvider(
		deps.Config,
		deps.Log,
		deps.Telemetry,
		deps.Status,
		deps.Host,
		sender,
		runner,
	)

	// If agent telemetry is enabled, add the start and stop hooks
	if atel.enabled {
		// Instruct FX to start and stop the agent telemetry
		deps.Lifecycle.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				return atel.start()
			},
			OnStop: func(ctx context.Context) error {
				return atel.stop()
			},
		})
	}

	return provides{Comp: atel}
}

func (atel *agenttelemetry) filterMetric(profile *Profile, promMetricFamily *dto.MetricFamily) *agentmetric {
	var amc *MetricConfig
	var ok bool

	// Check if the metric is included in the profile
	if amc, ok = profile.metricsMap[promMetricFamily.GetName()]; !ok {
		return nil
	}

	// Filter the metric according to the profile configuration
	// Currently we only support filtering out zero values if specified in the profile
	var filteredMetrics []*dto.Metric
	for _, m := range promMetricFamily.Metric {
		mt := promMetricFamily.GetType()

		// Filter out not supported types
		if !isSupportedMetricType(mt) {
			continue
		}

		// filter out zero values if specified in the profile
		if profile.excludeZeroMetric && isZeroValueMetric(mt, m) {
			continue
		}

		// filter out if contains excluded tags
		if len(profile.excludeTags) > 0 && areTagsMatching(m.GetLabel(), profile.excludeTags) {
			continue
		}

		// and now we can add the metric to the filtered metrics
		filteredMetrics = append(filteredMetrics, m)
	}

	// nothing to report
	if len(filteredMetrics) == 0 {
		return nil
	}

	return &agentmetric{
		metricName:       amc.Name,
		filteredMetrics:  filteredMetrics,
		promMetricFamily: promMetricFamily,
	}
}

func (atel *agenttelemetry) reportAgentMetrics(session *senderSession, profile *Profile) {
	// If no metrics are configured nothing to report
	if len(profile.metricsMap) == 0 {
		return
	}

	atel.logComp.Debugf("Collect Agent Metric telemetry for profile %s", profile.Name)

	// Gather all prom metrircs. Currently Gather() does not allow filtering by
	// matric name, so we need to gather all metrics and filter them on our own.
	//	pms, err := atel.telemetry.Gather(false)
	pms, err := atel.telComp.Gather(false)
	if err != nil {
		atel.logComp.Errorf("failed to get filtered telemetry metrics: %s", err)
		return
	}

	// ... and filter them according to the profile configuration
	var metrics []*agentmetric
	for _, pm := range pms {
		if am := atel.filterMetric(profile, pm); am != nil {
			metrics = append(metrics, am)
		}
	}

	// Send the metrics if any were filtered
	if len(metrics) == 0 {
		atel.logComp.Debug("No Agent Metric telemetry collected")
		return
	}

	// Send the metrics if any were filtered
	atel.logComp.Debugf("Reporting Agent Metric telemetry for profile %s", profile.Name)

	err = atel.sender.sendAgentMetricPayloads(session, metrics)
	if err != nil {
		atel.logComp.Errorf("failed to get filtered telemetry metrics: %s", err)
	}
}

func (atel *agenttelemetry) renderAgentStatus(profile *Profile, fullStatus map[string]interface{}, statusOutput map[string]interface{}) {
	// Render template if needed
	if profile.Status.Template != "none" {
		// Filter full Agent Status JSON via template which is selected by appending template suffix
		// to "pkg\status\render\templates\agent-telemetry-<suffix>.tmpl" file name. Currently we
		// support only "basic" suffix/template with future extensions to use other templates.
		statusBytes, err := render.FormatAgentTelemetry(fullStatus, profile.Status.Template)
		if err != nil {
			atel.logComp.Errorf("Failed to collect Agent Status telemetry. Error: %s", err.Error())
			return
		}
		if len(statusBytes) == 0 {
			atel.logComp.Debug("Agent status rendering to agent telemetry payloads return empty payload")
			return
		}

		// Convert byte slice to JSON object
		if err := json.Unmarshal(statusBytes, &statusOutput); err != nil {
			atel.logComp.Errorf("Failed to collect Agent Status telemetry. Error: %s", err.Error())
			return
		}
	}
}

func (atel *agenttelemetry) addAgentStatusExtra(profile *Profile, fullStatus map[string]interface{}, statusOutput map[string]interface{}) {
	for _, builder := range profile.statusExtraBuilder {
		// Evaluate JQ expression against the agent status JSON object
		jqResult := builder.jqSource.Run(fullStatus)
		jqValue, ok := jqResult.Next()
		if !ok {
			atel.logComp.Errorf("Failed to apply JQ expression for JSON path '%s' to Agent Status payload. Error unknown",
				strings.Join(builder.jpathTarget, "."))
			continue
		}

		// Validate JQ expression result
		if err, ok := jqValue.(error); ok {
			atel.logComp.Errorf("Failed to apply JQ expression for JSON path '%s' to Agent Status payload. Error: %s",
				strings.Join(builder.jpathTarget, "."), err.Error())
			continue
		}

		// Validate JQ expression result type
		var attrVal interface{}
		switch jqValueType := jqValue.(type) {
		case int:
			attrVal = jqValueType
		case float64:
			attrVal = jqValueType
		case bool:
			attrVal = jqValueType
		case nil:
			atel.logComp.Debugf("JQ expression return 'nil' value for JSON path '%s'", strings.Join(builder.jpathTarget, "."))
			continue
		case string:
			atel.logComp.Errorf("string value (%v) for JSON path '%s' for extra status atttribute is not currently allowed",
				strings.Join(builder.jpathTarget, "."), attrVal)
			continue
		default:
			atel.logComp.Errorf("'%v' value (%v) for JSON path '%s' for extra status atttribute is not currently allowed",
				reflect.TypeOf(jqValueType), reflect.ValueOf(jqValueType), strings.Join(builder.jpathTarget, "."))
			continue
		}

		// Add resulting value to the agent status telemetry payload (recursively creating missing JSON objects)
		curNode := statusOutput
		for i, p := range builder.jpathTarget {
			// last element is the attribute name
			if i == len(builder.jpathTarget)-1 {
				curNode[p] = attrVal
				break
			}

			existSubNode, ok := curNode[p]

			// if the node doesn't exist, create it
			if !ok {
				newSubNode := make(map[string]interface{})
				curNode[p] = newSubNode
				curNode = newSubNode
			} else {
				existSubNodeCasted, ok := existSubNode.(map[string]interface{})
				if !ok {
					atel.logComp.Errorf("JSON path '%s' points to non-object element", strings.Join(builder.jpathTarget[:i], "."))
					break
				}
				curNode = existSubNodeCasted
			}
		}
	}
}

func (atel *agenttelemetry) reportAgentStatus(session *senderSession, profile *Profile) {
	// If no status is configured nothing to report
	if profile.Status == nil {
		return
	}

	atel.logComp.Debugf("Collect Agent Status telemetry for profile %s", profile.Name)

	// Get Agent Status JSON object
	statusBytes, err := atel.statusComp.GetStatus("json", true)
	if err != nil {
		atel.logComp.Errorf("failed to get agent status: %s", err)
		return
	}

	var statusJSON = make(map[string]interface{})
	err = json.Unmarshal(statusBytes, &statusJSON)
	if err != nil {
		atel.logComp.Errorf("failed to unmarshall agent status: %s", err)
		return
	}

	// Render Agent Status JSON object (using template if needed and adding extra attributes)
	var statusPayloadJSON = make(map[string]interface{})
	atel.renderAgentStatus(profile, statusJSON, statusPayloadJSON)
	atel.addAgentStatusExtra(profile, statusJSON, statusPayloadJSON)
	if len(statusPayloadJSON) == 0 {
		atel.logComp.Debug("No Agent Status telemetry collected")
		return
	}

	atel.logComp.Debugf("Reporting Agent Status telemetry for profile %s", profile.Name)

	// Send the Agent Telemetry status payload
	err = atel.sender.sendAgentStatusPayload(session, statusPayloadJSON)
	if err != nil {
		atel.logComp.Errorf("failed to send agent status: %s", err)
		return
	}
}

// run runs the agent telemetry for a given profile. It is triggered by the runner
// according to the profiles schedule.
func (atel *agenttelemetry) run(profiles []*Profile) {
	session := atel.sender.startSession(atel.cancelCtx)

	for _, p := range profiles {
		atel.reportAgentMetrics(session, p)
		atel.reportAgentStatus(session, p)
	}

	err := atel.sender.flushSession(session)
	if err != nil {
		atel.logComp.Errorf("failed to flush agent telemetry session: %s", err)
		return
	}
}

// TODO: implement when CLI tool will be implemented
func (atel *agenttelemetry) GetAsJSON() ([]byte, error) {
	return nil, nil
}

// start is called by FX when the application starts.
func (atel *agenttelemetry) start() error {
	atel.logComp.Info("Starting agent telemetry")

	atel.cancelCtx, atel.cancel = context.WithCancel(context.Background())

	// Start the runner and add the jobs.
	atel.runner.start()
	for sh, pp := range atel.atelCfg.schedule {
		atel.runner.addJob(job{
			atel:     atel,
			profiles: pp,
			schedule: sh,
		})
	}

	return nil
}

// stop is called by FX when the application stops.
func (atel *agenttelemetry) stop() error {
	atel.logComp.Info("Stopping agent telemetry")
	atel.cancel()

	runnerCtx := atel.runner.stop()
	<-runnerCtx.Done()

	<-atel.cancelCtx.Done()
	atel.logComp.Info("Agent telemetry is stopped")
	return nil
}
