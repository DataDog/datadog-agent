// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package irgen

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/dyninst/exprlang"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/redaction"
)

// checkMetricProbeDefinition checks the parts of a metric probe's definition
// that don't depend on the binary and that rcjson.Validate, which the module
// runs before probes reach irgen, doesn't check: what Go supports. Problems are
// reported as issues so that the user sees an error status for the probe.
func checkMetricProbeDefinition(probe ir.ProbeDefinition) ir.Issue {
	invalid := func(msg string) ir.Issue {
		return ir.Issue{Kind: ir.IssueKindInvalidProbeDefinition, Message: msg}
	}
	mp, ok := probe.(ir.MetricProbeDefinition)
	if !ok {
		return invalid("metric probe definition does not describe a metric")
	}
	switch timing := strings.ToUpper(mp.GetEvaluateAt()); timing {
	case "", "DEFAULT", "ENTRY", "EXIT":
	default:
		return invalid(fmt.Sprintf("unsupported metric evaluation timing %q", timing))
	}
	_, valueJSON := mp.GetValue()
	switch kind := mp.GetMetricKind(); kind {
	case ir.MetricKindCount:
	case ir.MetricKindGauge:
		if len(valueJSON) == 0 {
			return invalid("gauge metric probes require a value expression")
		}
	case ir.MetricKindHistogram, ir.MetricKindDistribution:
		return ir.Issue{
			Kind:    ir.IssueKindUnsupportedFeature,
			Message: strings.ToLower(kind.String()) + " metric probes are not supported",
		}
	default:
		// rcjson.Validate rejects unknown kinds too, but a probe that skipped
		// it mustn't reach the compiler, which can't handle them.
		return invalid("metric kind is missing or unknown")
	}
	return ir.Issue{}
}

// analyzeMetricValue parses a metric probe's value expression and binds it to
// the event at which the probe fires. It returns nil and no issue when the
// probe has no value expression.
func analyzeMetricValue(
	mp ir.MetricProbeDefinition,
	inst *ir.ProbeInstance,
	varByName map[string]*ir.Variable,
	red *redaction.Config,
	haveEntry bool,
	entryIPs []ir.InjectionPoint,
	returnVars []*ir.Variable,
	returnDisplayNames map[*ir.Variable]string,
) (*analyzedExpression, ir.Issue) {
	dsl, valueJSON := mp.GetValue()
	if len(valueJSON) == 0 {
		return nil, ir.Issue{}
	}
	invalid := func(format string, args ...any) (*analyzedExpression, ir.Issue) {
		return nil, ir.Issue{
			Kind:    ir.IssueKindInvalidProbeDefinition,
			Message: fmt.Sprintf(format, args...),
		}
	}
	parsed, err := exprlang.Parse(valueJSON)
	if err != nil {
		return invalid("failed to parse metric value %q: %v", dsl, err)
	}
	if unsupported, ok := parsed.(*exprlang.UnsupportedExpr); ok {
		return nil, ir.Issue{
			Kind: ir.IssueKindUnsupportedFeature,
			Message: fmt.Sprintf(
				"metric value %q uses unsupported operation %q", dsl, unsupported.Operation,
			),
		}
	}
	rootName, ok := extractRootVariableName(parsed)
	if !ok {
		return invalid("metric value %q must reference a variable", dsl)
	}
	if name, ok := expressionReferencesRedacted(parsed, red); ok {
		return invalid("metric value references redacted identifier %q", name)
	}
	exit := metricEvaluatesAtReturn(mp)
	if (rootName == "@return" || rootName == "@duration") && !exit {
		return invalid("metric value %q uses %s, which is only available at function return", dsl, rootName)
	}
	rootVar := varByName[rootName]
	if rootName == "@return" && exit {
		var reason string
		rootVar, parsed, reason = rewriteReturnRefs(parsed, returnVars, returnDisplayNames)
		if reason != "" {
			return invalid("metric value %q: %s", dsl, reason)
		}
		if name, ok := expressionReferencesRedacted(parsed, red); ok {
			return invalid("metric value references redacted identifier %q", name)
		}
	}
	if rootVar == nil {
		return invalid("metric value %q references unknown variable %q", dsl, rootName)
	}
	var eventKind ir.EventKind
	switch {
	case exit:
		for _, event := range inst.Events {
			if event.Kind != ir.EventKindReturn {
				continue
			}
			if rootVar.Role != ir.VariableRoleDuration && !variableIsAvailable(event.InjectionPoints, rootVar) {
				return invalid("metric value %q references %q, which is not available at function return", dsl, rootName)
			}
			if isFloatType(rootVar) && floatIsRegisterOnly(event.InjectionPoints, rootVar) {
				return invalid("metric value %q references %q, which is in a floating-point register and can't be read at function return", dsl, rootName)
			}
		}
		eventKind = ir.EventKindReturn
	case haveEntry && rootVar.Role == ir.VariableRoleParameter:
		if isFloatType(rootVar) && floatIsRegisterOnly(entryIPs, rootVar) {
			return invalid(
				"metric value %q references %q, which is passed in a floating-point "+
					"register and can't be read at function entry",
				dsl, rootName,
			)
		}
		eventKind = ir.EventKindEntry
	case len(inst.Events) == 1 && inst.Events[0].Kind == ir.EventKindLine:
		ips := inst.Events[0].InjectionPoints
		if !variableIsAvailable(ips, rootVar) {
			return invalid(
				"metric value %q references %q, which is not available at this line",
				dsl, rootName,
			)
		}
		if isFloatType(rootVar) && floatIsRegisterOnly(ips, rootVar) {
			return invalid(
				"metric value %q references %q, which is in a floating-point "+
					"register and can't be read at this line",
				dsl, rootName,
			)
		}
		eventKind = ir.EventKindLine
	default:
		return invalid(
			"metric value %q references %q, which is not available at function entry",
			dsl, rootName,
		)
	}
	return &analyzedExpression{
		expr:         parsed,
		dsl:          dsl,
		rootVariable: rootVar,
		eventKind:    eventKind,
		exprKind:     ir.RootExpressionKindMetricValue,
		segmentIdx:   -1,
	}, ir.Issue{}
}

func metricEvaluatesAtReturn(probe ir.ProbeDefinition) bool {
	mp, ok := probe.(ir.MetricProbeDefinition)
	if !ok || !strings.EqualFold(mp.GetEvaluateAt(), "EXIT") {
		return false
	}
	_, function := probe.GetWhere().(ir.FunctionWhere)
	return function
}

func checkMetricGaugeTypes(probe *ir.Probe) ir.Issue {
	mp, ok := probe.ProbeDefinition.(ir.MetricProbeDefinition)
	if !ok || mp.GetMetricKind() != ir.MetricKindGauge {
		return ir.Issue{}
	}
	var first ir.Type
	for _, inst := range probe.Instances {
		for _, event := range inst.Events {
			for _, expr := range event.Type.Expressions {
				if expr.Kind != ir.RootExpressionKindMetricValue {
					continue
				}
				t := expr.Expression.Type
				if first == nil {
					first = t
				} else if metricGaugeEncoding(first) != metricGaugeEncoding(t) {
					return ir.Issue{
						Kind:    ir.IssueKindUnsupportedFeature,
						Message: fmt.Sprintf("incompatible gauge value types %q and %q across probe instances", first.GetName(), t.GetName()),
					}
				}
			}
		}
	}
	return ir.Issue{}
}

func metricGaugeEncoding(t ir.Type) reflect.Kind {
	kind, _ := t.GetGoKind()
	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return reflect.Int64
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr, reflect.Bool:
		return reflect.Uint64
	default:
		return kind
	}
}

func checkExpressionTypeRedaction(t ir.Type, tc *typeCatalog, red *redaction.Config) error {
	if !red.HasRedactedTypes() {
		return nil
	}
	seen := make(map[ir.TypeID]struct{})
	for t != nil {
		if canonical := tc.typesByID[t.GetID()]; canonical != nil {
			t = canonical
		}
		if red.RedactType(t.GetName()) {
			return fmt.Errorf("expression references redacted type %q", t.GetName())
		}
		if strings.Contains(t.GetName(), "go.shape.") {
			return fmt.Errorf("cannot check redacted types for generic type %q", t.GetName())
		}
		if _, ok := seen[t.GetID()]; ok {
			break
		}
		seen[t.GetID()] = struct{}{}
		ptr, ok := t.(*ir.PointerType)
		if !ok {
			break
		}
		t = ptr.Pointee
	}
	return nil
}

// checkMetricValueType checks that a metric probe's value expression has a
// type that the metric kind can report.
func checkMetricValueType(kind ir.MetricKind, t ir.Type) error {
	if _, duration := t.(*ir.DurationType); duration {
		if kind == ir.MetricKindGauge {
			return nil
		}
		return fmt.Errorf("@duration is measured in fractional milliseconds; use a gauge metric")
	}
	goKind, ok := t.GetGoKind()
	if !ok {
		return fmt.Errorf("value has type %s, which is not numeric", t.GetName())
	}
	switch goKind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Uintptr, reflect.Bool:
		return nil
	case reflect.Float32, reflect.Float64:
		if kind == ir.MetricKindGauge {
			return nil
		}
		return fmt.Errorf(
			"value has type %s; count metric probes need an integer or boolean value",
			t.GetName(),
		)
	default:
		return fmt.Errorf("value has type %s, which is not numeric", t.GetName())
	}
}
