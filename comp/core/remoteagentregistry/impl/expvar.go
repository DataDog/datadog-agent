// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remoteagentregistryimpl

import (
	"context"
	"expvar"
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

// remoteAgentsExpvarName is the top-level expvar key under which remote agent telemetry is exposed, keyed by the
// remote agent's sanitized display name, e.g. `remote_agents/agent-data-plane/forwarder/Transactions/Success`.
const remoteAgentsExpvarName = "remote_agents"

// remoteAgentExpvarCacheTTL bounds how often remote agents are queried when expvars are read. A single read of
// `/debug/vars` evaluates every mapped expvar, so this lets them all share one round of remote agent queries.
const remoteAgentExpvarCacheTTL = time.Second

// remoteAgentExpvarMappings maps telemetry metric families reported by remote agents to the Core Agent expvar path
// that tracks the same quantity. Values are summed across all label sets so they line up with the label-less expvars.
//
// Each mapped value is exposed both under `remote_agents/<agent>/<path>` and added into the Core Agent's own expvar at
// `<path>`, so that existing go_expvar configurations see the combined total across all processes.
var remoteAgentExpvarMappings = map[string][]string{
	"transactions__success": {"forwarder", "Transactions", "Success"},
}

var (
	expvarRegistry     atomic.Pointer[remoteAgentRegistry]
	publishExpvarsOnce sync.Once

	// wrappedExpvarsMu guards wrappedExpvars, the set of Core Agent expvar paths already wrapped to include remote
	// agent values.
	wrappedExpvarsMu sync.Mutex
	wrappedExpvars   = map[string]struct{}{}
)

// registerExpvars publishes remote agent telemetry as expvars. expvars are process-global and can only be published
// once, so the published functions read from whichever registry was started most recently.
func (ra *remoteAgentRegistry) registerExpvars() {
	expvarRegistry.Store(ra)
	publishExpvarsOnce.Do(func() {
		expvar.Publish(remoteAgentsExpvarName, expvar.Func(func() any {
			registry := expvarRegistry.Load()
			if registry == nil {
				return map[string]any{}
			}
			return registry.getRegisteredAgentsExpvars()
		}))
	})

	wrappedExpvarsMu.Lock()
	defer wrappedExpvarsMu.Unlock()
	for metricName, path := range remoteAgentExpvarMappings {
		key := strings.Join(path, "/")
		if _, wrapped := wrappedExpvars[key]; wrapped {
			continue
		}
		if wrapExpvarWithRemoteValues(path, metricName) {
			wrappedExpvars[key] = struct{}{}
		}
	}
}

// wrapExpvarWithRemoteValues replaces the Core Agent's *expvar.Int at path with an expvar.Func reporting the sum of
// the local value and the values reported by all remote agents for metricName. It returns false, leaving the expvar
// untouched, if the path does not exist in this process (e.g. the owning package is not linked in).
func wrapExpvarWithRemoteValues(path []string, metricName string) bool {
	parent, ok := expvar.Get(path[0]).(*expvar.Map)
	if !ok {
		return false
	}
	for _, key := range path[1 : len(path)-1] {
		if parent, ok = parent.Get(key).(*expvar.Map); !ok {
			return false
		}
	}

	leaf := path[len(path)-1]
	local, ok := parent.Get(leaf).(*expvar.Int)
	if !ok {
		log.Debugf("Not exposing remote agent values at expvar %q: expected an *expvar.Int", strings.Join(path, "/"))
		return false
	}

	parent.Set(leaf, expvar.Func(func() any {
		total := local.Value()
		if registry := expvarRegistry.Load(); registry != nil {
			for _, values := range registry.getRemoteAgentExpvarValues() {
				total += values[metricName]
			}
		}
		return total
	}))
	return true
}

// getRegisteredAgentsExpvars returns the mapped remote agent values as a nested map matching their expvar paths,
// keyed by the remote agent's sanitized display name.
func (ra *remoteAgentRegistry) getRegisteredAgentsExpvars() map[string]any {
	out := make(map[string]any)
	for agentName, values := range ra.getRemoteAgentExpvarValues() {
		agentVars := make(map[string]any)
		for metricName, value := range values {
			setNested(agentVars, remoteAgentExpvarMappings[metricName], value)
		}
		out[agentName] = agentVars
	}
	return out
}

type remoteAgentExpvarCache struct {
	mu        sync.Mutex
	fetchedAt time.Time
	values    map[string]map[string]int64
}

// getRemoteAgentExpvarValues returns, for each remote agent that answered, the summed value of each mapped metric
// family it reported. Results are cached for remoteAgentExpvarCacheTTL.
func (ra *remoteAgentRegistry) getRemoteAgentExpvarValues() map[string]map[string]int64 {
	cache := &ra.expvarCache
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.values != nil && time.Since(cache.fetchedAt) < remoteAgentExpvarCacheTTL {
		return cache.values
	}

	client := func(ctx context.Context, remoteAgent *remoteAgentClient, opts ...grpc.CallOption) (*pb.GetTelemetryResponse, error) {
		return remoteAgent.GetTelemetry(ctx, &pb.GetTelemetryRequest{}, opts...)
	}
	type agentValues struct {
		name   string
		values map[string]int64
	}
	processor := func(details remoteagentregistry.RegisteredAgent, resp *pb.GetTelemetryResponse, err error) agentValues {
		if err != nil {
			log.Warnf("Failed to collect expvars from remoteAgent %v: %v", details.SanitizedDisplayName, err)
			return agentValues{name: details.SanitizedDisplayName}
		}
		promText, ok := resp.Payload.(*pb.GetTelemetryResponse_PromText)
		if !ok {
			return agentValues{name: details.SanitizedDisplayName}
		}
		return agentValues{name: details.SanitizedDisplayName, values: expvarValuesFromPromText(promText.PromText, details.SanitizedDisplayName)}
	}

	values := make(map[string]map[string]int64)
	for _, agent := range callAgentsForService(ra, TelemetryServiceName, client, processor) {
		if agent.values != nil {
			values[agent.name] = agent.values
		}
	}

	cache.values = values
	cache.fetchedAt = time.Now()
	return values
}

// expvarValuesFromPromText sums each metric family listed in remoteAgentExpvarMappings across all of its label sets.
func expvarValuesFromPromText(promText string, remoteAgentName string) map[string]int64 {
	parser := expfmt.NewTextParser(model.LegacyValidation)
	metricFamilies, err := parser.TextToMetricFamilies(strings.NewReader(promText))
	if err != nil {
		log.Warnf("Failed to parse prometheus text from remoteAgent %v: %v", remoteAgentName, err)
		return nil
	}

	out := make(map[string]int64)
	for name := range remoteAgentExpvarMappings {
		mf, found := metricFamilies[name]
		if !found {
			continue
		}

		var total float64
		for _, metric := range mf.GetMetric() {
			switch mf.GetType() {
			case dto.MetricType_COUNTER:
				total += metric.GetCounter().GetValue()
			case dto.MetricType_GAUGE:
				total += metric.GetGauge().GetValue()
			default:
				log.Debugf("Skipping expvar for metric %v from remoteAgent %v: unsupported metric type %s", name, remoteAgentName, mf.GetType())
			}
		}
		out[name] = int64(total)
	}
	return out
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
