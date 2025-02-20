// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package sampler

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-go/v5/statsd"
)

const (
	// These constants exist to match the behavior of the OTEL probabilistic sampler.
	// See: https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/6229c6ad1c49e9cc4b41a8aab8cb5a94a7b82ea5/processor/probabilisticsamplerprocessor/tracesprocessor.go#L38-L42
	numProbabilisticBuckets = 0x4000
	bitMaskHashBuckets      = numProbabilisticBuckets - 1
	percentageScaleFactor   = numProbabilisticBuckets / 100.0

	// probRateKey indicates the percentage sampling rate configured for the probabilistic sampler
	probRateKey = "_dd.prob_sr"

	// MetricsProbabilisticSamplerSamplingRulesEvaluations is the metric name for the number of probabilistic sampler rules evaluated.
	MetricsProbabilisticSamplerSamplingRulesEvaluations = "datadog.trace_agent.sampler.probabilistic.sampling_rule.evaluations"
	// MetricsProbabilisticSamplerSamplingRuleMatches is the metric name for the number of probabilistic sampler rules that matched.
	MetricsProbabilisticSamplerSamplingRuleMatches = "datadog.trace_agent.sampler.probabilistic.sampling_rule.matches"
)

// ProbabilisticSampler is a sampler that overrides all other samplers,
// it deterministically samples incoming traces by a hash of their trace ID
type ProbabilisticSampler struct {
	enabled                  bool
	hashSeed                 []byte
	scaledSamplingPercentage uint32
	samplingPercentage       float64
	// If any rules don't match the span, it fallbacks to the `samplingPercentage`.
	// traceSamplingRules is a list of rules that can be used to override the `samplingPercentage`.
	traceSamplingRules       probabilisticSamplerRules
	samplingRuleMetrics      map[string]probabilisticSamplerRuleMetrics
	samplingRuleMetricsMutex sync.Mutex
	// fullTraceIDMode looks at the full 128-bit trace ID to make the sampling decision
	// This can be useful when trying to run this probabilistic sampler alongside the
	// OTEL probabilistic sampler processor which always looks at the full 128-bit trace id.
	// This is disabled by default to ensure compatibility in distributed systems where legacy applications may
	// drop the top 64 bits of the trace ID.
	fullTraceIDMode bool
}

type probabilisticSamplerRule struct {
	service          *regexp.Regexp
	operationName    *regexp.Regexp
	resourceName     *regexp.Regexp
	attributes       map[string]*regexp.Regexp
	scaledPercentage uint32
	percentage       float64
}

// String returns a string representation of the probabilisticSamplerRule.
func (r *probabilisticSamplerRule) String() string {
	var b strings.Builder
	if r.service != nil {
		b.WriteString(fmt.Sprintf("service=%s, ", r.service.String()))
	}
	if r.operationName != nil {
		b.WriteString(fmt.Sprintf("operation_name=%s, ", r.operationName.String()))
	}
	if r.resourceName != nil {
		b.WriteString(fmt.Sprintf("resource_name=%s, ", r.resourceName.String()))
	}
	if len(r.attributes) > 0 {
		b.WriteString("attributes=[")
		for k, v := range r.attributes {
			b.WriteString(fmt.Sprintf("%s=%s, ", k, v.String()))
		}
		b.WriteString("], ")
	}
	b.WriteString(fmt.Sprintf("percentage=%f", r.percentage))
	return b.String()
}

func (r *probabilisticSamplerRule) evaluate(root *trace.Span) (matched, evaluated bool) {
	if r.service != nil {
		evaluated = true
		if !r.service.MatchString(root.Service) {
			return false, true
		}
	}
	if r.operationName != nil {
		evaluated = true
		if !r.operationName.MatchString(root.Name) {
			return false, true
		}
	}
	if r.resourceName != nil {
		evaluated = true
		if !r.resourceName.MatchString(root.Resource) {
			return false, true
		}
	}
	for tagKey, tagRegex := range r.attributes {
		if tagRegex == nil {
			continue
		}
		if root.Meta != nil {
			evaluated = true
			if v, ok := root.Meta[tagKey]; ok && tagRegex.MatchString(v) {
				continue
			}
		}
		if root.Metrics != nil {
			evaluated = true
			v, ok := root.Metrics[tagKey]
			// sampling on numbers with floating point is not supported,
			// thus 'math.Floor(v) != v'
			if !ok || math.Floor(v) != v || !tagRegex.MatchString(strconv.FormatFloat(v, 'g', -1, 64)) {
				return false, true
			}
		}
	}
	return true, evaluated
}

func compileProbabilisticSamplerRules(rules []config.ProbabilisticSamplerRule) (probabilisticSamplerRules, error) {
	compiledRules := make(probabilisticSamplerRules, len(rules))
	for i, rule := range rules {
		var err error
		if rule.Service != "" {
			compiledRules[i].service, err = regexp.Compile(rule.Service)
			if err != nil {
				return nil, fmt.Errorf("service regex: %w", err)
			}
		}
		if rule.OperationName != "" {
			compiledRules[i].operationName, err = regexp.Compile(rule.OperationName)
			if err != nil {
				return nil, fmt.Errorf("name regex: %w", err)
			}
		}
		if rule.ResourceName != "" {
			compiledRules[i].resourceName, err = regexp.Compile(rule.ResourceName)
			if err != nil {
				return nil, fmt.Errorf("resource regex: %w", err)
			}
		}
		compiledRules[i].attributes = make(map[string]*regexp.Regexp, len(rule.Attributes))
		for k, v := range rule.Attributes {
			compiledRules[i].attributes[k], err = regexp.Compile(v)
			if err != nil {
				return nil, fmt.Errorf("tag regex: key=%s, value=%s: %w", k, v, err)
			}
		}
		compiledRules[i].scaledPercentage = uint32(rule.Percentage * percentageScaleFactor)
		compiledRules[i].percentage = float64(rule.Percentage) / 100.
	}
	return compiledRules, nil
}

type probabilisticSamplerRules []probabilisticSamplerRule

// String returns a string representation of the probabilisticSamplerRules.
func (rs probabilisticSamplerRules) String() string {
	rules := make([]string, len(rs))
	for i, rule := range rs {
		rules[i] = fmt.Sprintf("rule %d: %s", i, rule.String())
	}
	return strings.Join(rules, ", ")
}

type probabilisticSamplerRuleMetrics struct {
	evaluations int64
	matches     int64
}

// NewProbabilisticSampler returns a new ProbabilisticSampler that deterministically samples
// a given percentage of incoming spans based on their trace ID
func NewProbabilisticSampler(conf *config.AgentConfig) *ProbabilisticSampler {
	hashSeedBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(hashSeedBytes, conf.ProbabilisticSamplerHashSeed)
	_, fullTraceIDMode := conf.Features["probabilistic_sampler_full_trace_id"]
	rules, err := compileProbabilisticSamplerRules(conf.ProbabilisticSamplerTraceSamplingRules)
	if err == nil && len(rules) > 0 {
		log.Infof("Compiled probabilistic sampler trace sampling rules: %s", rules.String())
	} else {
		log.Errorf("Compiling probabilistic sampler trace sampling rules: %v", err)
	}
	return &ProbabilisticSampler{
		enabled:                  conf.ProbabilisticSamplerEnabled,
		hashSeed:                 hashSeedBytes,
		scaledSamplingPercentage: uint32(conf.ProbabilisticSamplerSamplingPercentage * percentageScaleFactor),
		samplingPercentage:       float64(conf.ProbabilisticSamplerSamplingPercentage) / 100.,
		traceSamplingRules:       rules,
		samplingRuleMetrics:      make(map[string]probabilisticSamplerRuleMetrics),
		fullTraceIDMode:          fullTraceIDMode,
	}
}

func (ps *ProbabilisticSampler) percentage(root *trace.Span) (uint32, float64) {
	var matched, evaluated bool
	defer func() {
		if !evaluated {
			return
		}
		ps.samplingRuleMetricsMutex.Lock()
		metrics := ps.samplingRuleMetrics[root.Service]
		metrics.evaluations++
		if matched {
			metrics.matches++
		}
		ps.samplingRuleMetrics[root.Service] = metrics
		ps.samplingRuleMetricsMutex.Unlock()
	}()
	for _, rule := range ps.traceSamplingRules {
		matched, evaluated = rule.evaluate(root)
		if matched && evaluated {
			return rule.scaledPercentage, rule.percentage
		}
	}
	return ps.scaledSamplingPercentage, ps.samplingPercentage
}

func (ps *ProbabilisticSampler) report(statsd statsd.ClientInterface) {
	if !ps.enabled || len(ps.traceSamplingRules) == 0 {
		return
	}
	ps.samplingRuleMetricsMutex.Lock()
	defer ps.samplingRuleMetricsMutex.Unlock()
	for service, metrics := range ps.samplingRuleMetrics {
		tags := []string{"rule_type:trace", "target_service:" + service}
		_ = statsd.Count(MetricsProbabilisticSamplerSamplingRulesEvaluations, metrics.evaluations, tags, 1)
		_ = statsd.Count(MetricsProbabilisticSamplerSamplingRuleMatches, metrics.matches, tags, 1)
	}
	ps.samplingRuleMetrics = make(map[string]probabilisticSamplerRuleMetrics)
}

// Sample a trace given the chunk's root span, returns true if the trace should be kept
func (ps *ProbabilisticSampler) Sample(root *trace.Span) bool {
	if !ps.enabled {
		return false
	}

	tid := make([]byte, 16)
	var err error
	if !ps.fullTraceIDMode {
		binary.BigEndian.PutUint64(tid, root.TraceID)
	} else {
		tid, err = get128BitTraceID(root)
	}
	if err != nil {
		log.Errorf("Unable to probabilistically sample, failed to determine 128-bit trace ID from incoming span: %v", err)
		return false
	}

	hasher := fnv.New32a()
	_, _ = hasher.Write(ps.hashSeed)
	_, _ = hasher.Write(tid)
	hash := hasher.Sum32()
	scaledSamplingPercentage, samplingPercentage := ps.percentage(root)
	keep := hash&bitMaskHashBuckets < scaledSamplingPercentage
	if keep {
		setMetric(root, probRateKey, samplingPercentage)
	}
	return keep
}

func get128BitTraceID(span *trace.Span) ([]byte, error) {
	// If it's an otel span the whole trace ID is in otel.trace
	if tid, ok := span.Meta["otel.trace_id"]; ok {
		bs, err := hex.DecodeString(tid)
		if err != nil {
			return nil, err
		}
		return bs, nil
	}
	tid := make([]byte, 16)
	binary.BigEndian.PutUint64(tid[8:], span.TraceID)
	// Get hex encoded upper bits for datadog spans
	// If no value is found we can use the default `0` value as that's what will have been propagated
	if upper, ok := span.Meta["_dd.p.tid"]; ok {
		u, err := strconv.ParseUint(upper, 16, 64)
		if err != nil {
			return nil, err
		}
		binary.BigEndian.PutUint64(tid[:8], u)
	}
	return tid, nil
}
