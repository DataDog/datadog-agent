// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remoteagentregistryimpl

import (
	"bytes"
	"context"
	"encoding/json"
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
	// Retire origin-labeled series after this long without appearing in a successful telemetry scrape.
	remoteExpvarOriginIdleTTL = 10 * time.Minute
)

// expvarMapping describes one compatibility mapping from a RAR Prometheus metric family to a legacy expvar path.
// The metric names are the names emitted by ADP's RAR remapping, not the names from ADP's direct /compat/metrics
// endpoint.
type expvarMapping struct {
	id              string
	metricName      string
	expvarPath      []string
	selectors       []labelSelector
	metricType      dto.MetricType
	createIfMissing bool
}

type labelSelector struct {
	name   string
	values map[string]struct{}
}

var expvarMappings = []expvarMapping{
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
		id:         "dogstatsd_uds_origin_detection_errors",
		metricName: "dogstatsd__uds_origin_detection_errors",
		expvarPath: []string{"dogstatsd-uds", "OriginDetectionErrors"},
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
	{
		id:         "forwarder_retry_queue_size",
		metricName: "transactions__retry_queue_size",
		expvarPath: []string{"forwarder", "Transactions", "RetryQueueSize"},
		metricType: dto.MetricType_GAUGE,
	},
	{
		id:         "forwarder_success",
		metricName: "transactions__success",
		expvarPath: []string{"forwarder", "Transactions", "Success"},
		metricType: dto.MetricType_COUNTER,
	},
	{
		id:         "forwarder_errors",
		metricName: "transactions__errors",
		expvarPath: []string{"forwarder", "Transactions", "Errors"},
		metricType: dto.MetricType_COUNTER,
	},
	{
		id:         "forwarder_dns_errors",
		metricName: "transactions__errors",
		expvarPath: []string{"forwarder", "Transactions", "ErrorsByType", "DNSErrors"},
		selectors:  selectors(labelSelectorValues("error_type", "dns_lookup_failure")),
		metricType: dto.MetricType_COUNTER,
	},
	{
		id:         "forwarder_tls_errors",
		metricName: "transactions__errors",
		expvarPath: []string{"forwarder", "Transactions", "ErrorsByType", "TLSErrors"},
		selectors:  selectors(labelSelectorValues("error_type", "tls_handshake_failure")),
		metricType: dto.MetricType_COUNTER,
	},
	{
		id:         "forwarder_connection_errors",
		metricName: "transactions__errors",
		expvarPath: []string{"forwarder", "Transactions", "ErrorsByType", "ConnectionErrors"},
		selectors:  selectors(labelSelectorValues("error_type", "connection_failure")),
		metricType: dto.MetricType_COUNTER,
	},
	{
		id:         "forwarder_wrote_request_errors",
		metricName: "transactions__errors",
		expvarPath: []string{"forwarder", "Transactions", "ErrorsByType", "WroteRequestErrors"},
		selectors:  selectors(labelSelectorValues("error_type", "writing_failure")),
		metricType: dto.MetricType_COUNTER,
	},
	{
		id:         "forwarder_sent_request_errors",
		metricName: "transactions__errors",
		expvarPath: []string{"forwarder", "Transactions", "ErrorsByType", "SentRequestErrors"},
		selectors:  selectors(labelSelectorValues("error_type", "invalid_request")),
		metricType: dto.MetricType_COUNTER,
	},
	{
		id:         "forwarder_http_errors",
		metricName: "transactions__http_errors",
		expvarPath: []string{"forwarder", "Transactions", "HTTPErrors"},
		metricType: dto.MetricType_COUNTER,
	},
	{
		id:         "aggregator_dogstatsd_contexts",
		metricName: "aggregator__dogstatsd_contexts",
		expvarPath: []string{"aggregator", "DogstatsdContexts"},
		metricType: dto.MetricType_GAUGE,
	},
	{
		id:         "aggregator_dogstatsd_metric_sample",
		metricName: "aggregator__processed",
		expvarPath: []string{"aggregator", "DogstatsdMetricSample"},
		selectors:  selectors(labelSelectorValues("data_type", "dogstatsd_metrics")),
		metricType: dto.MetricType_COUNTER,
	},
	{
		id:         "aggregator_series_flushed",
		metricName: "aggregator__flush",
		expvarPath: []string{"aggregator", "SeriesFlushed"},
		selectors:  selectors(labelSelectorValues("data_type", "series")),
		metricType: dto.MetricType_COUNTER,
	},
	{
		id:         "aggregator_event",
		metricName: "aggregator__processed",
		expvarPath: []string{"aggregator", "Event"},
		selectors:  selectors(labelSelectorValues("data_type", "events")),
		metricType: dto.MetricType_COUNTER,
	},
	{
		id:         "aggregator_service_check",
		metricName: "aggregator__processed",
		expvarPath: []string{"aggregator", "ServiceCheck"},
		selectors:  selectors(labelSelectorValues("data_type", "service_checks")),
		metricType: dto.MetricType_COUNTER,
	},
	{
		id:         "aggregator_events_flushed",
		metricName: "aggregator__flush",
		expvarPath: []string{"aggregator", "EventsFlushed"},
		selectors:  selectors(labelSelectorValues("data_type", "events")),
		metricType: dto.MetricType_COUNTER,
	},
	{
		id:         "aggregator_service_check_flushed",
		metricName: "aggregator__flush",
		expvarPath: []string{"aggregator", "ServiceCheckFlushed"},
		selectors:  selectors(labelSelectorValues("data_type", "service_checks")),
		metricType: dto.MetricType_COUNTER,
	},
	{
		id:         "aggregator_number_of_flush",
		metricName: "aggregator__number_of_flush",
		expvarPath: []string{"aggregator", "NumberOfFlush"},
		metricType: dto.MetricType_COUNTER,
	},
	{
		id:         "aggregator_flush_count_series",
		metricName: "aggregator__flush_count",
		expvarPath: []string{"aggregator", "FlushCount", "Series", "LastFlush"},
		selectors:  selectors(labelSelectorValues("flush_type", "series")),
		metricType: dto.MetricType_GAUGE,
	},
	{
		id:         "aggregator_flush_count_sketches",
		metricName: "aggregator__flush_count",
		expvarPath: []string{"aggregator", "FlushCount", "Sketches", "LastFlush"},
		selectors:  selectors(labelSelectorValues("flush_type", "sketches")),
		metricType: dto.MetricType_GAUGE,
	},
	{
		id:         "aggregator_flush_count_events",
		metricName: "aggregator__flush_count",
		expvarPath: []string{"aggregator", "FlushCount", "Events", "LastFlush"},
		selectors:  selectors(labelSelectorValues("flush_type", "events")),
		metricType: dto.MetricType_GAUGE,
	},
	{
		id:         "aggregator_flush_count_service_checks",
		metricName: "aggregator__flush_count",
		expvarPath: []string{"aggregator", "FlushCount", "ServiceChecks", "LastFlush"},
		selectors:  selectors(labelSelectorValues("flush_type", "service_checks")),
		metricType: dto.MetricType_GAUGE,
	},
	{
		id:         "aggregator_main_flush_time",
		metricName: "aggregator__flush_time",
		expvarPath: []string{"aggregator", "Flush", "MainFlushTime", "LastFlush"},
		selectors:  selectors(labelSelectorValues("flush_type", "main")),
		metricType: dto.MetricType_GAUGE,
	},
	{
		id:         "aggregator_event_flush_time",
		metricName: "aggregator__flush_time",
		expvarPath: []string{"aggregator", "Flush", "EventFlushTime", "LastFlush"},
		selectors:  selectors(labelSelectorValues("flush_type", "event")),
		metricType: dto.MetricType_GAUGE,
	},
	{
		id:         "aggregator_service_check_flush_time",
		metricName: "aggregator__flush_time",
		expvarPath: []string{"aggregator", "Flush", "ServiceCheckFlushTime", "LastFlush"},
		selectors:  selectors(labelSelectorValues("flush_type", "service_check")),
		metricType: dto.MetricType_GAUGE,
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
	wrappedExpvarFuncsMu sync.Mutex
	wrappedExpvarFuncs   = make(map[string]*dynamicExpvarWrapper)
)

// Aggregator expvar subtrees such as FlushCount and Flush are snapshots exposed as expvar.Func,
// not mutable expvar.Maps. Wrap each snapshot once and overlay remote values at mapped leaves.
type dynamicExpvarMapping struct {
	id   string
	path []string
}

type dynamicExpvarWrapper struct {
	original expvar.Func
	mu       sync.RWMutex
	mappings []dynamicExpvarMapping
}

func (wrapper *dynamicExpvarWrapper) addMapping(mapping dynamicExpvarMapping) {
	wrapper.mu.Lock()
	defer wrapper.mu.Unlock()
	mapping.path = append([]string(nil), mapping.path...)
	wrapper.mappings = append(wrapper.mappings, mapping)
}

func (wrapper *dynamicExpvarWrapper) value() any {
	local := wrapper.original()
	wrapper.mu.RLock()
	mappings := append([]dynamicExpvarMapping(nil), wrapper.mappings...)
	wrapper.mu.RUnlock()

	registry := remoteExpvarRegistry.Load()
	if registry == nil {
		return local
	}
	return mergeDynamicExpvarValues(local, mappings, registry.getRemoteExpvarValues())
}

func mergeDynamicExpvarValues(local any, mappings []dynamicExpvarMapping, remoteValues map[string]map[string]int64) any {
	if len(remoteValues) == 0 || len(mappings) == 0 {
		return local
	}
	encoded, err := json.Marshal(local)
	if err != nil {
		return local
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return local
	}

	for _, mapping := range mappings {
		if len(mapping.path) == 0 {
			continue
		}
		var remote int64
		for _, agentValues := range remoteValues {
			remote = saturatingAdd(remote, agentValues[mapping.id])
		}
		if remote == 0 {
			continue
		}

		parent, ok := value.(map[string]any)
		if !ok {
			continue
		}
		for _, key := range mapping.path[:len(mapping.path)-1] {
			parent, ok = parent[key].(map[string]any)
			if !ok {
				break
			}
		}
		if !ok {
			continue
		}
		leaf := mapping.path[len(mapping.path)-1]
		localNumber, ok := parent[leaf].(json.Number)
		if !ok {
			continue
		}
		localValue, err := localNumber.Int64()
		if err != nil {
			continue
		}
		parent[leaf] = saturatingAdd(localValue, remote)
	}
	return value
}

type remoteExpvarCache struct {
	mu        sync.Mutex
	fetchedAt time.Time
	values    map[string]map[string]int64
	counters  map[remoteSeriesKey]*remoteCounterState
	gauges    map[remoteSeriesKey]int64
	// retiredTotals preserves cumulative expvar values after their per-series counter state is deleted.
	// Its keys are the reporting dimensions (agent name and fixed mapping ID), not series labels.
	retiredTotals map[string]map[string]int64
}

type remoteSeriesKey struct {
	agentIdentity string
	agentName     string
	mappingID     string
	labels        string
}

type remoteCounterState struct {
	initialized bool
	lastRaw     int64
	total       int64
	lastSeen    time.Time
	hasOrigin   bool
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
	for _, mapping := range expvarMappings {
		key := strings.Join(mapping.expvarPath, "/")
		if _, wrapped := wrappedExpvars[key]; wrapped {
			continue
		}
		if wrapExpvar(mapping) {
			wrappedExpvars[key] = struct{}{}
		}
	}
}

// Increment ExpVar quantity
func wrapExpvar(mapping expvarMapping) bool {
	if wrapDynamicExpvar(mapping) {
		return true
	}
	if len(mapping.expvarPath) < 2 {
		return false
	}

	rootName := mapping.expvarPath[0]
	root := expvar.Get(rootName)
	parent, ok := root.(*expvar.Map)
	if root == nil && mapping.createIfMissing {
		parent = expvar.NewMap(rootName)
		ok = true
	}
	if !ok {
		return false
	}
	for _, key := range mapping.expvarPath[1 : len(mapping.expvarPath)-1] {
		child := parent.Get(key)
		if child == nil && mapping.createIfMissing {
			childMap := new(expvar.Map).Init()
			parent.Set(key, childMap)
			parent = childMap
			continue
		}
		parent, ok = child.(*expvar.Map)
		if !ok {
			return false
		}
	}

	leaf := mapping.expvarPath[len(mapping.expvarPath)-1]
	leafVar := parent.Get(leaf)
	local, ok := leafVar.(*expvar.Int)
	if leafVar == nil && mapping.createIfMissing {
		local = new(expvar.Int)
		parent.Set(leaf, local)
		ok = true
	}
	if !ok {
		log.Warnf("Not exposing remote agent values at expvar %q: expected an *expvar.Int", strings.Join(mapping.expvarPath, "/"))
		return false
	}

	// Wrap the local leaf so reads return the combined Core Agent and remote ADP value.
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

func wrapDynamicExpvar(mapping expvarMapping) bool {
	if len(mapping.expvarPath) < 3 {
		return false
	}

	root, ok := expvar.Get(mapping.expvarPath[0]).(*expvar.Map)
	if !ok {
		return false
	}
	parent := root
	for index := 1; index < len(mapping.expvarPath)-1; index++ {
		key := mapping.expvarPath[index]
		child := parent.Get(key)
		function, isFunction := child.(expvar.Func)
		if isFunction {
			prefix := strings.Join(mapping.expvarPath[:index+1], "/")
			wrappedExpvarFuncsMu.Lock()
			wrapper := wrappedExpvarFuncs[prefix]
			if wrapper == nil {
				wrapper = &dynamicExpvarWrapper{original: function}
				wrappedExpvarFuncs[prefix] = wrapper
				parent.Set(key, expvar.Func(wrapper.value))
			}
			wrapper.addMapping(dynamicExpvarMapping{
				id:   mapping.id,
				path: mapping.expvarPath[index+1:],
			})
			wrappedExpvarFuncsMu.Unlock()
			return true
		}

		childMap, isMap := child.(*expvar.Map)
		if !isMap {
			return false
		}
		parent = childMap
	}
	return false
}

// getRemoteAgentExpvars formats remote compatibility values as nested expvar data grouped by remote-agent name.
// The values are obtained through the shared cache and do not include local Core Agent values.
func (ra *remoteAgentRegistry) getRemoteAgentExpvars() map[string]any {
	values := make(map[string]any)
	for agentName, agentValues := range ra.getRemoteExpvarValues() {
		agentVars := make(map[string]any)
		for _, mapping := range expvarMappings {
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

// Retrieves Expvar values from cache
func (ra *remoteAgentRegistry) getRemoteExpvarValues() map[string]map[string]int64 {
	cache := &ra.expvarCache
	cache.mu.Lock()
	defer cache.mu.Unlock()

	// Return early to avoid unnecessary updates and allocations
	if cache.values != nil && time.Since(cache.fetchedAt) < remoteExpvarCacheTTL {
		return cache.values
	}

	// Instantiate counters if they're not currently present
	if cache.counters == nil {
		cache.counters = make(map[remoteSeriesKey]*remoteCounterState)
	}
	if cache.gauges == nil {
		cache.gauges = make(map[remoteSeriesKey]int64)
	}
	if cache.retiredTotals == nil {
		cache.retiredTotals = make(map[string]map[string]int64)
	}

	// Update values since, presumably, more time has elapsed than we'd feel comforatble caching for
	values := collectRemoteExpvarValuesAt(ra, cache, time.Now())
	cache.values = values
	cache.fetchedAt = time.Now()
	return values
}

// Collect Prometheus endpoint response from RAR for ADP and convert to Expvar.
func collectRemoteExpvarValuesAt(ra *remoteAgentRegistry, cache *remoteExpvarCache, now time.Time) map[string]map[string]int64 {
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
		series, parsed := expvarSeriesFromPromText(payload.PromText)
		if !parsed {
			return remoteExpvarAgentResponse{}
		}
		return remoteExpvarAgentResponse{
			agent:   details,
			series:  series,
			success: true,
		}
	}

	responses := callAgentsForServiceFiltered(ra, TelemetryServiceName, isADPAgent, client, processor)
	successfulAgents := make(map[string]struct{})
	activeAgents := make(map[string]struct{})
	for _, agent := range ra.GetRegisteredAgents() {
		if isADPAgent(agent) {
			activeAgents[remoteAgentIdentity(agent)] = struct{}{}
		}
	}
	for _, response := range responses {
		if response.agent.SanitizedDisplayName == "" {
			continue
		}
		identity := remoteAgentIdentity(response.agent)
		if response.success {
			successfulAgents[identity] = struct{}{}
			// Gauges are the latest successful snapshot, unlike counters which are accumulated.
			for key := range cache.gauges {
				if key.agentIdentity == identity {
					delete(cache.gauges, key)
				}
			}
		}
		for _, series := range response.series {
			key := remoteSeriesKey{
				agentIdentity: identity,
				agentName:     response.agent.SanitizedDisplayName,
				mappingID:     series.mappingID,
				labels:        series.labels,
			}
			if series.metricType == dto.MetricType_GAUGE {
				cache.gauges[key] = series.value
				continue
			}
			state := cache.counters[key]
			if state == nil {
				state = &remoteCounterState{}
				cache.counters[key] = state
			}
			state.observe(series.value)
			state.lastSeen = now
			// The origin label is optional and is emitted when dogstatsd_origin is enabled.
			// Only origin-labeled series use absence-based expiry; otherwise PID unregistration
			// is the lifecycle signal for retiring per-series state.
			state.hasOrigin = series.hasOrigin
		}
	}

	values := make(map[string]map[string]int64)
	addValue := func(agentName, mappingID string, value int64) {
		if values[agentName] == nil {
			values[agentName] = make(map[string]int64)
		}
		values[agentName][mappingID] = saturatingAdd(values[agentName][mappingID], value)
	}
	for agentName, totals := range cache.retiredTotals {
		for mappingID, total := range totals {
			addValue(agentName, mappingID, total)
		}
	}
	for key, state := range cache.counters {
		_, registered := activeAgents[key.agentIdentity]
		_, scrapeSucceeded := successfulAgents[key.agentIdentity]
		originSeriesExpired := state.hasOrigin && scrapeSucceeded && now.Sub(state.lastSeen) >= remoteExpvarOriginIdleTTL
		if !registered || originSeriesExpired {
			addRetiredExpvarTotal(cache.retiredTotals, key.agentName, key.mappingID, state.total)
			addValue(key.agentName, key.mappingID, state.total)
			delete(cache.counters, key)
			continue
		}
		if state.initialized {
			addValue(key.agentName, key.mappingID, state.total)
		}
	}
	for key, value := range cache.gauges {
		if _, registered := activeAgents[key.agentIdentity]; !registered {
			delete(cache.gauges, key)
			continue
		}
		addValue(key.agentName, key.mappingID, value)
	}
	return values
}

type remoteExpvarAgentResponse struct {
	agent   remoteagentregistry.RegisteredAgent
	series  []remoteExpvarSeries
	success bool
}

type remoteExpvarSeries struct {
	mappingID  string
	metricType dto.MetricType
	labels     string
	hasOrigin  bool
	value      int64
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

func (s *remoteCounterState) observe(raw int64) bool {
	if !s.initialized {
		s.initialized = true
		s.lastRaw = raw
		s.total = raw
		return true
	}

	changed := raw != s.lastRaw
	if raw >= s.lastRaw {
		s.total = saturatingAdd(s.total, raw-s.lastRaw)
	} else {
		// A decrease means the remote process or its counter was reset. Preserve the
		// accumulated value and add the post-reset counter value.
		s.total = saturatingAdd(s.total, raw)
	}
	s.lastRaw = raw
	return changed
}

func addRetiredExpvarTotal(totals map[string]map[string]int64, agentName, mappingID string, value int64) {
	if totals[agentName] == nil {
		totals[agentName] = make(map[string]int64)
	}
	totals[agentName][mappingID] = saturatingAdd(totals[agentName][mappingID], value)
}

// Map Prometheus metrics to ExpVar format
func expvarSeriesFromPromText(promText string) ([]remoteExpvarSeries, bool) {
	parser := expfmt.NewTextParser(model.LegacyValidation)
	metricFamilies, err := parser.TextToMetricFamilies(strings.NewReader(promText))
	if err != nil {
		log.Warnf("Failed to parse Prometheus text for expvar compatibility: %v", err)
		return nil, false
	}

	var series []remoteExpvarSeries
	for _, mapping := range expvarMappings {
		family, ok := metricFamilies[mapping.metricName]
		if !ok || family.GetType() != mapping.metricType {
			continue
		}
		for _, metric := range family.GetMetric() {
			if metric == nil || !matchesSelectors(metric.GetLabel(), mapping.selectors) {
				continue
			}
			value, ok := metricValue(metric, mapping.metricType)
			if !ok {
				continue
			}
			series = append(series, remoteExpvarSeries{
				mappingID:  mapping.id,
				metricType: mapping.metricType,
				labels:     canonicalLabels(metric.GetLabel()),
				hasOrigin:  hasLabel(metric.GetLabel(), "origin"),
				value:      value,
			})
		}
	}
	return series, true
}

func metricValue(metric *dto.Metric, metricType dto.MetricType) (int64, bool) {
	switch metricType {
	case dto.MetricType_COUNTER:
		return counterValue(metric)
	case dto.MetricType_GAUGE:
		return gaugeValue(metric)
	default:
		return 0, false
	}
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

func gaugeValue(metric *dto.Metric) (int64, bool) {
	if metric == nil || metric.Gauge == nil {
		return 0, false
	}
	value := metric.Gauge.GetValue()
	if math.IsNaN(value) || math.IsInf(value, 0) || value != math.Trunc(value) || value >= float64(math.MaxInt64) || value < -float64(math.MaxInt64)-1 {
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

func hasLabel(labels []*dto.LabelPair, name string) bool {
	for _, label := range labels {
		if label != nil && label.GetName() == name {
			return true
		}
	}
	return false
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
