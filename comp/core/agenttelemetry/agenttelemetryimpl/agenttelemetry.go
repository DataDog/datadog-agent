// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agenttelemetryimpl provides the implementation of the agenttelemetry component.
package agenttelemetryimpl

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/agenttelemetry"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/status/render"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	dto "github.com/prometheus/client_model/go"
	"go.uber.org/fx"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newAtel))
}

type atel struct {
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

// Interfacing with runner.
type job struct {
	a        *atel
	profiles []*Profile
	schedule Schedule
}

func (j job) Run() {
	j.a.run(j.profiles)
}

// Passing metrics to sender Interfacing with sender
type agentmetric struct {
	metricName       string
	filteredMetrics  []*dto.Metric
	promMetricFamily *dto.MetricFamily
}

func createSender(
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

func createAtel(
	cfgComp config.Component,
	logComp log.Component,
	telComp telemetry.Component,
	statusComp status.Component,
	hostComp host.Component,
	sender sender,
	runner runner) *atel {
	// Parse Agent Telemetry Configuration configuration
	atelCfg, err := parseConfig(cfgComp)
	if err != nil {
		logComp.Errorf("Failed to parse agent telemetry config: %s", err.Error())
		return &atel{}
	}
	if !atelCfg.Enabled {
		logComp.Info("Agent telemetry is disabled")
		return &atel{}
	}

	return &atel{
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

func newAtel(deps dependencies) agenttelemetry.Component {
	sender, err := createSender(deps.Config, deps.Log, deps.Host)
	if err != nil {
		return &atel{}
	}

	runner := newRunnerImpl()

	// Wire up the agent telemetry provider (TODO: use FX for sender, client and runner?)
	a := createAtel(
		deps.Config,
		deps.Log,
		deps.Telemetry,
		deps.Status,
		deps.Host,
		sender,
		runner,
	)

	// If agent telemetry is enabled, add the start and stop hooks
	if a.enabled {
		// Instruct FX to start and stop the agent telemetry
		deps.Lifecycle.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				return a.start()
			},
			OnStop: func(ctx context.Context) error {
				return a.stop()
			},
		})
	}

	return a
}

func (a *atel) filterMetric(profile *Profile, promMetricFamily *dto.MetricFamily) *agentmetric {
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

func (a *atel) reportAgentMetrics(session *senderSession, profile *Profile) {
	// If no metrics are configured nothing to report
	if len(profile.metricsMap) == 0 {
		return
	}

	a.logComp.Debugf("Collect Agent Metric telemetry for profile %s", profile.Name)

	// Gather all prom metrircs. Currently Gather() does not allow filtering by
	// matric name, so we need to gather all metrics and filter them on our own.
	//	pms, err := a.telemetry.Gather(false)
	pms, err := a.telComp.Gather(false)
	if err != nil {
		a.logComp.Errorf("failed to get filtered telemetry metrics: %s", err)
		return
	}

	// ... and filter them according to the profile configuration
	var metrics []*agentmetric
	for _, pm := range pms {
		if am := a.filterMetric(profile, pm); am != nil {
			metrics = append(metrics, am)
		}
	}

	// Send the metrics if any were filtered
	if len(metrics) == 0 {
		a.logComp.Debug("No Agent Metric telemetry collected")
		return
	}

	// Send the metrics if any were filtered
	a.logComp.Debugf("Reporting Agent Metric telemetry for profile %s", profile.Name)

	err = a.sender.sendAgentMetricPayloads(session, metrics)
	if err != nil {
		a.logComp.Errorf("failed to get filtered telemetry metrics: %s", err)
	}
}

func (a *atel) renderAgentStatus(profile *Profile, fullStatus map[string]interface{}, statusOutput map[string]interface{}) {
	// Render template if needed
	if profile.Status.Template != "none" {
		// Filter full Agent Status JSON via template which is selected by appending template suffix
		// to "pkg\status\render\templates\agent-telemetry-<suffix>.tmpl" file name. Currently we
		// support only "basic" suffix/template with future extensions to use other templates.
		statusBytes, err := render.FormatAgentTelemetry(fullStatus, profile.Status.Template)
		if err != nil {
			a.logComp.Errorf("Failed to collect Agent Status telemetry. Error: %s", err.Error())
			return
		}
		if len(statusBytes) == 0 {
			a.logComp.Debug("Agent status rendering to agent telemetry payloads return empty payload")
			return
		}

		// Convert byte slice to JSON object
		if err := json.Unmarshal(statusBytes, &statusOutput); err != nil {
			a.logComp.Errorf("Failed to collect Agent Status telemetry. Error: %s", err.Error())
			return
		}
	}
}

func (a *atel) addAgentStatusExtra(profile *Profile, fullStatus map[string]interface{}, statusOutput map[string]interface{}) {
	for _, builder := range profile.statusExtraBuilder {
		// Evaluate JQ expression against the agent status JSON object
		jqResult := builder.jqSource.Run(fullStatus)
		jqValue, ok := jqResult.Next()
		if !ok {
			a.logComp.Errorf("Failed to apply JQ expression for JSON path '%s' to Agent Status payload. Error unknown",
				strings.Join(builder.jpathTarget, "."))
			continue
		}

		// Validate JQ expression result
		if err, ok := jqValue.(error); ok {
			a.logComp.Errorf("Failed to apply JQ expression for JSON path '%s' to Agent Status payload. Error: %s",
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
			a.logComp.Debugf("JQ expression return 'nil' value for JSON path '%s'", strings.Join(builder.jpathTarget, "."))
			continue
		case string:
			a.logComp.Errorf("string value (%v) for JSON path '%s' for extra status atttribute is not currently allowed",
				strings.Join(builder.jpathTarget, "."), attrVal)
			continue
		default:
			a.logComp.Errorf("'%v' value (%v) for JSON path '%s' for extra status atttribute is not currently allowed",
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
					a.logComp.Errorf("JSON path '%s' points to non-object element", strings.Join(builder.jpathTarget[:i], "."))
					break
				}
				curNode = existSubNodeCasted
			}
		}
	}
}

func (a *atel) reportAgentStatus(session *senderSession, profile *Profile) {
	// If no status is configured nothing to report
	if profile.Status == nil {
		return
	}

	a.logComp.Debugf("Collect Agent Status telemetry for profile %s", profile.Name)

	// Get Agent Status JSON object
	statusBytes, err := a.statusComp.GetStatus("json", true)
	if err != nil {
		a.logComp.Errorf("failed to get agent status: %s", err)
		return
	}

	var statusJSON = make(map[string]interface{})
	err = json.Unmarshal(statusBytes, &statusJSON)
	if err != nil {
		a.logComp.Errorf("failed to unmarshall agent status: %s", err)
		return
	}

	// Render Agent Status JSON object (using template if needed and adding extra attributes)
	var statusPayloadJSON = make(map[string]interface{})
	a.renderAgentStatus(profile, statusJSON, statusPayloadJSON)
	a.addAgentStatusExtra(profile, statusJSON, statusPayloadJSON)
	if len(statusPayloadJSON) == 0 {
		a.logComp.Debug("No Agent Status telemetry collected")
		return
	}

	a.logComp.Debugf("Reporting Agent Status telemetry for profile %s", profile.Name)

	// Send the Agent Telemetry status payload
	err = a.sender.sendAgentStatusPayload(session, statusPayloadJSON)
	if err != nil {
		a.logComp.Errorf("failed to send agent status: %s", err)
		return
	}
}

// run runs the agent telemetry for a given profile. It is triggered by the runner
// according to the profiles schedule.
func (a *atel) run(profiles []*Profile) {
	session := a.sender.startSession(a.cancelCtx)

	for _, p := range profiles {
		a.reportAgentMetrics(session, p)
		a.reportAgentStatus(session, p)
	}

	err := a.sender.flushSession(session)
	if err != nil {
		a.logComp.Errorf("failed to flush agent telemetry session: %s", err)
		return
	}
}

// TODO: implement when CLI tool will be implemented
func (a *atel) GetAsJSON() ([]byte, error) {
	return nil, nil
}

// start is called by FX when the application starts.
func (a *atel) start() error {
	a.logComp.Info("Starting agent telemetry")

	a.cancelCtx, a.cancel = context.WithCancel(context.Background())

	// Start the runner and add the jobs.
	a.runner.start()
	for sh, pp := range a.atelCfg.schedule {
		a.runner.addJob(job{
			a:        a,
			profiles: pp,
			schedule: sh,
		})
	}

	return nil
}

// stop is called by FX when the application stops.
func (a *atel) stop() error {
	a.logComp.Info("Stopping agent telemetry")
	a.cancel()

	runnerCtx := a.runner.stop()
	<-runnerCtx.Done()

	<-a.cancelCtx.Done()
	a.logComp.Info("Agent telemetry is stopped")
	return nil
}
