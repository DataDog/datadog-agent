// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remoteagentregistryimpl

import (
	"context"
	"expvar"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"google.golang.org/grpc"

	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	remoteAgentsExpvarName = "remote_agents"
	adpAgentFlavor         = "agent_data_plane"
	remoteExpvarCacheTTL   = time.Second
)

// expvarMapping describes one compatibility mapping from a RAR Prometheus metric family to a legacy expvar path.
// The metric names are the names emitted by ADP's RAR remapping, not the names from ADP's direct /compat/metrics
// endpoint.
type expvarMapping struct {
	id         string
	metricName string
	expvarPath []string
	selectors  []labelSelector
	metricType dto.MetricType
}

type labelSelector struct {
	name   string
	values map[string]struct{}
}

var dogstatsdExpvarMappings = []expvarMapping{
	{
		id:         "dogstatsd_udp_packets",
		metricName: "dogstatsd__udp_packets",
		expvarPath: []string{"dogstatsd-udp", "Packets"},
		metricType: dto.MetricType_COUNTER,
	},
	{
		id:         "dogstatsd_udp_packet_reading_errors",
		metricName: "dogstatsd__udp_packets",
		expvarPath: []string{"dogstatsd-udp", "PacketReadingErrors"},
		selectors:  selectors(labelSelectorValues("state", "error")),
		metricType: dto.MetricType_COUNTER,
	},
	{
		id:         "dogstatsd_uds_packets",
		metricName: "dogstatsd__uds_packets",
		expvarPath: []string{"dogstatsd-uds", "Packets"},
		selectors:  selectors(labelSelectorValues("transport", "unix", "unixgram")),
		metricType: dto.MetricType_COUNTER,
	},
	{
		id:         "dogstatsd_uds_packet_reading_errors",
		metricName: "dogstatsd__uds_packets",
		expvarPath: []string{"dogstatsd-uds", "PacketReadingErrors"},
		selectors: selectors(
			labelSelectorValues("transport", "unix", "unixgram"),
			labelSelectorValues("state", "error"),
		),
		metricType: dto.MetricType_COUNTER,
	},
	{
		id:         "dogstatsd_metric_packets",
		metricName: "dogstatsd__processed",
		expvarPath: []string{"dogstatsd", "MetricPackets"},
		selectors: selectors(
			labelSelectorValues("message_type", "metrics"),
			labelSelectorValues("state", "ok"),
		),
		metricType: dto.MetricType_COUNTER,
	},
	{
		id:         "dogstatsd_metric_parse_errors",
		metricName: "dogstatsd__processed",
		expvarPath: []string{"dogstatsd", "MetricParseErrors"},
		selectors: selectors(
			labelSelectorValues("message_type", "metrics"),
			labelSelectorValues("state", "error"),
		),
		metricType: dto.MetricType_COUNTER,
	},
	{
		id:         "dogstatsd_event_packets",
		metricName: "dogstatsd__processed",
		expvarPath: []string{"dogstatsd", "EventPackets"},
		selectors: selectors(
			labelSelectorValues("message_type", "events"),
			labelSelectorValues("state", "ok"),
		),
		metricType: dto.MetricType_COUNTER,
	},
	{
		id:         "dogstatsd_event_parse_errors",
		metricName: "dogstatsd__processed",
		expvarPath: []string{"dogstatsd", "EventParseErrors"},
		selectors: selectors(
			labelSelectorValues("message_type", "events"),
			labelSelectorValues("state", "error"),
		),
		metricType: dto.MetricType_COUNTER,
	},
	{
		id:         "dogstatsd_service_check_packets",
		metricName: "dogstatsd__processed",
		expvarPath: []string{"dogstatsd", "ServiceCheckPackets"},
		selectors: selectors(
			labelSelectorValues("message_type", "service_checks"),
			labelSelectorValues("state", "ok"),
		),
		metricType: dto.MetricType_COUNTER,
	},
	{
		id:         "dogstatsd_service_check_parse_errors",
		metricName: "dogstatsd__processed",
		expvarPath: []string{"dogstatsd", "ServiceCheckParseErrors"},
		selectors: selectors(
			labelSelectorValues("message_type", "service_checks"),
			labelSelectorValues("state", "error"),
		),
		metricType: dto.MetricType_COUNTER,
	},
}

func labelSelectorValues(name string, values ...string) labelSelector {
	allowed := make(map[string]struct{}, len(values))
	for _, value := range values {
		allowed[value] = struct{}{}
	}
	return labelSelector{name: name, values: allowed}
}

func selectors(values ...labelSelector) []labelSelector {
	return values
}

var (
	remoteExpvarRegistry atomic.Pointer[remoteAgentRegistry]
	publishRemoteExpvars sync.Once
	wrappedExpvarsMu     sync.Mutex
	wrappedExpvars       = make(map[string]struct{})
)

type remoteExpvarCache struct {
	mu        sync.Mutex
	fetchedAt time.Time
	values    map[string]map[string]int64
	counters  map[remoteCounterKey]*remoteCounterState
}

type remoteCounterKey struct {
	agentIdentity string
	agentName     string
	mappingID     string
	labels        string
}

type remoteCounterState struct {
	initialized bool
	lastRaw     int64
	total       int64
}

// registerExpvars publishes remote agent values and wraps compatible local expvars. The Go expvar registry is global,
// so the published functions resolve the registry that is active when they are evaluated.
func (ra *remoteAgentRegistry) registerExpvars() {
	remoteExpvarRegistry.Store(ra)
	publishRemoteExpvars.Do(func() {
		if expvar.Get(remoteAgentsExpvarName) != nil {
			return
		}
		expvar.Publish(remoteAgentsExpvarName, expvar.Func(func() any {
			registry := remoteExpvarRegistry.Load()
			if registry == nil {
				return map[string]any{}
			}
			return registry.getRemoteAgentExpvars()
		}))
	})

	wrappedExpvarsMu.Lock()
	defer wrappedExpvarsMu.Unlock()
	for _, mapping := range dogstatsdExpvarMappings {
		key := strings.Join(mapping.expvarPath, "/")
		if _, wrapped := wrappedExpvars[key]; wrapped {
			continue
		}
		if wrapExpvar(mapping) {
			wrappedExpvars[key] = struct{}{}
		}
	}
}

func wrapExpvar(mapping expvarMapping) bool {
	if len(mapping.expvarPath) < 2 {
		return false
	}

	parent, ok := expvar.Get(mapping.expvarPath[0]).(*expvar.Map)
	if !ok {
		return false
	}
	for _, key := range mapping.expvarPath[1 : len(mapping.expvarPath)-1] {
		parent, ok = parent.Get(key).(*expvar.Map)
		if !ok {
			return false
		}
	}

	leaf := mapping.expvarPath[len(mapping.expvarPath)-1]
	local, ok := parent.Get(leaf).(*expvar.Int)
	if !ok {
		log.Debugf("Not exposing remote agent values at expvar %q: expected an *expvar.Int", strings.Join(mapping.expvarPath, "/"))
		return false
	}

	parent.Set(leaf, expvar.Func(func() any {
		total := local.Value()
		if registry := remoteExpvarRegistry.Load(); registry != nil {
			for _, values := range registry.getRemoteExpvarValues() {
				total = saturatingAdd(total, values[mapping.id])
			}
		}
		return total
	}))
	return true
}

func (ra *remoteAgentRegistry) getRemoteAgentExpvars() map[string]any {
	values := make(map[string]any)
	for agentName, agentValues := range ra.getRemoteExpvarValues() {
		agentVars := make(map[string]any)
		for _, mapping := range dogstatsdExpvarMappings {
			if value, ok := agentValues[mapping.id]; ok {
				setNested(agentVars, mapping.expvarPath, value)
			}
		}
		if len(agentVars) > 0 {
			values[agentName] = agentVars
		}
	}
	return values
}

func (ra *remoteAgentRegistry) getRemoteExpvarValues() map[string]map[string]int64 {
	cache := &ra.expvarCache
	cache.mu.Lock()
	defer cache.mu.Unlock()

	if cache.values != nil && time.Since(cache.fetchedAt) < remoteExpvarCacheTTL {
		return cache.values
	}
	if cache.counters == nil {
		cache.counters = make(map[remoteCounterKey]*remoteCounterState)
	}

	values := collectRemoteExpvarValues(ra, cache.counters)
	cache.values = values
	cache.fetchedAt = time.Now()
	return values
}

func collectRemoteExpvarValues(ra *remoteAgentRegistry, counters map[remoteCounterKey]*remoteCounterState) map[string]map[string]int64 {
	client := func(ctx context.Context, remoteAgent *remoteAgentClient, opts ...grpc.CallOption) (*pb.GetTelemetryResponse, error) {
		return remoteAgent.GetTelemetry(ctx, &pb.GetTelemetryRequest{}, opts...)
	}
	processor := func(details remoteagentregistry.RegisteredAgent, resp *pb.GetTelemetryResponse, err error) remoteExpvarAgentResponse {
		if err != nil {
			log.Warnf("Failed to collect expvars from remoteAgent %v: %v", details.SanitizedDisplayName, err)
			return remoteExpvarAgentResponse{}
		}
		payload, ok := resp.Payload.(*pb.GetTelemetryResponse_PromText)
		if !ok {
			return remoteExpvarAgentResponse{}
		}
		return remoteExpvarAgentResponse{
			agent:  details,
			series: expvarSeriesFromPromText(payload.PromText),
		}
	}

	responses := callAgentsForServiceFiltered(ra, TelemetryServiceName, isADPAgent, client, processor)
	for _, response := range responses {
		if response.agent.SanitizedDisplayName == "" {
			continue
		}
		for _, series := range response.series {
			key := remoteCounterKey{
				agentIdentity: remoteAgentIdentity(response.agent),
				agentName:     response.agent.SanitizedDisplayName,
				mappingID:     series.mappingID,
				labels:        series.labels,
			}
			state := counters[key]
			if state == nil {
				state = &remoteCounterState{}
				counters[key] = state
			}
			state.observe(series.value)
		}
	}

	values := make(map[string]map[string]int64)
	for key, state := range counters {
		if !state.initialized {
			continue
		}
		if values[key.agentName] == nil {
			values[key.agentName] = make(map[string]int64)
		}
		values[key.agentName][key.mappingID] = saturatingAdd(values[key.agentName][key.mappingID], state.total)
	}
	return values
}

type remoteExpvarAgentResponse struct {
	agent  remoteagentregistry.RegisteredAgent
	series []remoteExpvarSeries
}

type remoteExpvarSeries struct {
	mappingID string
	labels    string
	value     int64
}

func isADPAgent(agent remoteagentregistry.RegisteredAgent) bool {
	return agent.Flavor == adpAgentFlavor
}

func remoteAgentIdentity(agent remoteagentregistry.RegisteredAgent) string {
	identity := agent.Flavor + "\x00" + agent.SanitizedDisplayName
	if agent.PID != "" {
		identity += "\x00" + agent.PID
	}
	return identity
}

func (s *remoteCounterState) observe(raw int64) {
	if !s.initialized {
		s.initialized = true
		s.lastRaw = raw
		s.total = raw
		return
	}

	if raw >= s.lastRaw {
		s.total = saturatingAdd(s.total, raw-s.lastRaw)
	} else {
		// A decrease means the remote process or its counter was reset. Preserve the
		// accumulated value and add the post-reset counter value.
		s.total = saturatingAdd(s.total, raw)
	}
	s.lastRaw = raw
}

func expvarSeriesFromPromText(promText string) []remoteExpvarSeries {
	parser := expfmt.NewTextParser(model.LegacyValidation)
	metricFamilies, err := parser.TextToMetricFamilies(strings.NewReader(promText))
	if err != nil {
		log.Warnf("Failed to parse Prometheus text for expvar compatibility: %v", err)
		return nil
	}

	var series []remoteExpvarSeries
	for _, mapping := range dogstatsdExpvarMappings {
		family, ok := metricFamilies[mapping.metricName]
		if !ok || family.GetType() != mapping.metricType {
			continue
		}
		for _, metric := range family.GetMetric() {
			if metric == nil || !matchesSelectors(metric.GetLabel(), mapping.selectors) {
				continue
			}
			value, ok := counterValue(metric)
			if !ok {
				continue
			}
			series = append(series, remoteExpvarSeries{
				mappingID: mapping.id,
				labels:    canonicalLabels(metric.GetLabel()),
				value:     value,
			})
		}
	}
	return series
}

func counterValue(metric *dto.Metric) (int64, bool) {
	if metric == nil || metric.Counter == nil {
		return 0, false
	}
	value := metric.Counter.GetValue()
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value != math.Trunc(value) || value >= float64(math.MaxInt64) {
		return 0, false
	}
	return int64(value), true
}

func matchesSelectors(labels []*dto.LabelPair, selectors []labelSelector) bool {
	values := make(map[string]string, len(labels))
	for _, label := range labels {
		if label != nil {
			values[label.GetName()] = label.GetValue()
		}
	}
	for _, selector := range selectors {
		value, ok := values[selector.name]
		if !ok {
			return false
		}
		if _, ok := selector.values[value]; !ok {
			return false
		}
	}
	return true
}

func canonicalLabels(labels []*dto.LabelPair) string {
	pairs := make([]string, 0, len(labels))
	for _, label := range labels {
		if label != nil {
			pairs = append(pairs, label.GetName()+"="+label.GetValue())
		}
	}
	sort.Strings(pairs)
	return strings.Join(pairs, "\x00")
}

func saturatingAdd(left, right int64) int64 {
	if right > 0 && left > math.MaxInt64-right {
		return math.MaxInt64
	}
	if right < 0 && left < math.MinInt64-right {
		return math.MinInt64
	}
	return left + right
}

func setNested(root map[string]any, path []string, value any) {
	node := root
	for _, key := range path[:len(path)-1] {
		child, ok := node[key].(map[string]any)
		if !ok {
			child = make(map[string]any)
			node[key] = child
		}
		node = child
	}
	node[path[len(path)-1]] = value
}
