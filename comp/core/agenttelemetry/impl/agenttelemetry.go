// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// ---------------------------------------------------
//
// This is experimental code and is subject to change.
//
// ---------------------------------------------------

// Package agenttelemetryimpl provides the implementation of the agenttelemetry component.
package agenttelemetryimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"golang.org/x/exp/maps"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"

	dto "github.com/prometheus/client_model/go"
)

type atel struct {
	cfgComp config.Component
	logComp log.Component
	telComp telemetry.Component

	enabled bool
	sender  sender
	runner  runner
	atelCfg *Config

	cancelCtx context.Context
	cancel    context.CancelFunc
}

type Provides struct {
	compdef.Out

	Comp     agenttelemetry.Component
	Endpoint api.AgentEndpointProvider
}

type Requires struct {
	compdef.In

	Log       log.Component
	Config    config.Component
	Telemetry telemetry.Component

	Lc compdef.Lifecycle
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
	name    string
	metrics []*dto.Metric
	family  *dto.MetricFamily
}

func createSender(
	cfgComp config.Component,
	logComp log.Component,
) (sender, error) {
	client := newSenderClientImpl(cfgComp)
	sender, err := newSenderImpl(cfgComp, logComp, client)
	if err != nil {
		logComp.Errorf("Failed to create agent telemetry sender: %s", err.Error())
	}
	return sender, err
}

func createAtel(
	cfgComp config.Component,
	logComp log.Component,
	telComp telemetry.Component,
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

	if sender == nil {
		sender, err = createSender(cfgComp, logComp)
		if err != nil {
			logComp.Errorf("Failed to create agent telemetry sender: %s", err.Error())
			return &atel{}
		}
	}

	if runner == nil {
		runner = newRunnerImpl()
	}

	return &atel{
		enabled: true,
		cfgComp: cfgComp,
		logComp: logComp,
		telComp: telComp,
		sender:  sender,
		runner:  runner,
		atelCfg: atelCfg,
	}
}

func NewComponent(deps Requires) Provides {
	a := createAtel(
		deps.Config,
		deps.Log,
		deps.Telemetry,
		nil,
		nil,
	)

	// If agent telemetry is enabled and configured properly add the start and stop hooks
	if a.enabled {
		deps.Lc.Append(compdef.Hook{
			OnStart: func(_ context.Context) error {
				return a.start()
			},
			OnStop: func(_ context.Context) error {
				return a.stop()
			},
		})
	}

	return Provides{
		Comp:     a,
		Endpoint: api.NewAgentEndpointProvider(a.writePayload, "/metadata/agent-telemetry", "GET"),
	}
}

func (a *atel) aggregateMetricTags(mCfg *MetricConfig, mt dto.MetricType, ms []*dto.Metric) []*dto.Metric {
	// Nothing to aggregate?
	if len(ms) == 0 {
		return nil
	}

	// Special case when no aggregate tags are defined - aggregate all metrics
	// aggregateMetric will sum all metrics into a single one without copying tags
	if !mCfg.aggregateTagsExists {
		ma := &dto.Metric{}
		for _, m := range ms {
			aggregateMetric(mt, ma, m)
		}

		return []*dto.Metric{ma}
	}

	amMap := make(map[string]*dto.Metric)

	// Initialize total metric
	var totalm *dto.Metric
	if mCfg.AggregateTotal {
		totalm = &dto.Metric{}
	}

	// Enumerate the metric's timeseries and aggregate them
	for _, m := range ms {
		tagsKey := ""

		// if tags are defined, we need to create a key from them by dropping not specified
		// in configuration tags. The key is constructed by conatenating specified tag names and values
		// if the a timeseries has tags is not specified in
		origTags := m.GetLabel()
		if len(origTags) > 0 {
			// sort tags (to have a consistent key for the same tag set)
			tags := cloneLabelsSorted(origTags)

			// create a key from the tags (and drop not specified in the configuration tags)
			var specTags = make([]*dto.LabelPair, 0, len(origTags))
			for _, t := range tags {
				if _, ok := mCfg.aggregateTagsMap[t.GetName()]; ok {
					specTags = append(specTags, t)
					tagsKey += makeLabelPairKey(t)
				}
			}
			if mCfg.AggregateTotal {
				aggregateMetric(mt, totalm, m)
			}

			// finally aggregate the metric on the created key
			if aggm, ok := amMap[tagsKey]; ok {
				aggregateMetric(mt, aggm, m)
			} else {
				// ... or create a new one with specifi value and specified tags
				aggm := &dto.Metric{}
				aggregateMetric(mt, aggm, m)
				aggm.Label = specTags
				amMap[tagsKey] = aggm
			}
		} else {
			// if no tags are specified, we aggregate all metrics into a single one
			if mCfg.AggregateTotal {
				aggregateMetric(mt, totalm, m)
			}
		}
	}

	// Add total metric if needed
	if mCfg.AggregateTotal {
		totalName := "total"
		totalValue := strconv.Itoa(len(ms))
		totalm.Label = []*dto.LabelPair{
			{Name: &totalName, Value: &totalValue},
		}
		amMap[totalName] = totalm
	}

	// Anything to report?
	if len(amMap) == 0 {
		return nil
	}

	// Convert the map to a slice
	return maps.Values(amMap)
}

func isMetricFiltered(p *Profile, mCfg *MetricConfig, mt dto.MetricType, m *dto.Metric) bool {
	// filter out zero values if specified in the profile
	if p.excludeZeroMetric && isZeroValueMetric(mt, m) {
		return false
	}

	// filter out if contains excluded tags
	if len(p.excludeTagsMap) > 0 && areTagsMatching(m.GetLabel(), p.excludeTagsMap) {
		return false
	}

	// filter out if tag does not contain in existing aggregateTags
	if mCfg.aggregateTagsExists && !areTagsMatching(m.GetLabel(), mCfg.aggregateTagsMap) {
		return false
	}

	return true
}

func (a *atel) transformMetricFamily(p *Profile, mfam *dto.MetricFamily) *agentmetric {
	var mCfg *MetricConfig
	var ok bool

	// Check if the metric is included in the profile
	if mCfg, ok = p.metricsMap[mfam.GetName()]; !ok {
		return nil
	}

	// Filter out not supported types
	mt := mfam.GetType()
	if !isSupportedMetricType(mt) {
		return nil
	}

	// Filter the metric according to the profile configuration
	// Currently we only support filtering out zero values if specified in the profile
	var fm []*dto.Metric
	for _, m := range mfam.Metric {
		if isMetricFiltered(p, mCfg, mt, m) {
			fm = append(fm, m)
		}
	}

	amt := a.aggregateMetricTags(mCfg, mt, fm)

	// nothing to report
	if len(fm) == 0 {
		return nil
	}

	return &agentmetric{
		name:    mCfg.Name,
		metrics: amt,
		family:  mfam,
	}
}

func (a *atel) reportAgentMetrics(session *senderSession, pms []*telemetry.MetricFamily, p *Profile) {
	// If no metrics are configured nothing to report
	if len(p.metricsMap) == 0 {
		return
	}

	a.logComp.Debugf("Collect Agent Metric telemetry for profile %s", p.Name)

	// ... and filter them according to the profile configuration
	var metrics []*agentmetric
	for _, pm := range pms {
		if am := a.transformMetricFamily(p, pm); am != nil {
			metrics = append(metrics, am)
		}
	}

	// Send the metrics if any were filtered
	if len(metrics) == 0 {
		a.logComp.Debug("No Agent Metric telemetry collected")
		return
	}

	// Send the metrics if any were filtered
	a.logComp.Debugf("Reporting Agent Metric telemetry for profile %s", p.Name)

	a.sender.sendAgentMetricPayloads(session, metrics)
}

func (a *atel) loadPayloads(profiles []*Profile) (*senderSession, error) {
	// Gather all prom metrircs. Currently Gather() does not allow filtering by
	// matric name, so we need to gather all metrics and filter them on our own.
	//	pms, err := a.telemetry.Gather(false)
	pms, err := a.telComp.Gather(false)
	if err != nil {
		a.logComp.Errorf("failed to get filtered telemetry metrics: %v", err)
		return nil, err
	}

	session := a.sender.startSession(a.cancelCtx)
	for _, p := range profiles {
		a.reportAgentMetrics(session, pms, p)
	}
	return session, nil
}

// run runs the agent telemetry for a given profile. It is triggered by the runner
// according to the profiles schedule.
func (a *atel) run(profiles []*Profile) {
	a.logComp.Info("Starting agent telemetry run")

	session, err := a.loadPayloads(profiles)
	if err != nil {
		a.logComp.Errorf("failed to load agent telemetry session: %s", err)
		return
	}

	err = a.sender.flushSession(session)
	if err != nil {
		a.logComp.Errorf("failed to flush agent telemetry session: %s", err)
		return
	}
}

func (a *atel) writePayload(w http.ResponseWriter, _ *http.Request) {
	if !a.enabled {
		httputils.SetJSONError(w, errors.New("agent-telemetry is not enabled. Please enable agent telemetry"), 400)
		return
	}

	a.logComp.Info("Dumping agent telemetry payload")

	payload, err := a.GetAsJSON()
	if err != nil {
		httputils.SetJSONError(w, a.logComp.Error(err.Error()), 500)
		return
	}

	w.Write(payload)
}

func (a *atel) GetAsJSON() ([]byte, error) {
	session, err := a.loadPayloads(a.atelCfg.Profiles)
	if err != nil {
		return nil, fmt.Errorf("unable to load agent telemetry payload: %w", err)
	}
	payload := session.flush()

	jsonPayload, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("unable to marshal agent telemetry payload: %w", err)
	}

	jsonPayloadScrubbed, err := scrubber.ScrubBytes(jsonPayload)
	if err != nil {
		return nil, fmt.Errorf("unable to scrub agent telemetry payload: %w", err)
	}

	return jsonPayloadScrubbed, nil
}

// start is called by FX when the application starts.
func (a *atel) start() error {
	a.logComp.Infof("Starting agent telemetry for %d schedules and %d profiles", len(a.atelCfg.schedule), len(a.atelCfg.Profiles))

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
