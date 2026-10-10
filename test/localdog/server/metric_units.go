// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"strings"

	"github.com/DataDog/datadog-agent/test/localdog/store"
)

// The agent doesn't send units for most core metrics: Datadog gets them from metric metadata.
// These are the units of well-known metrics, as {unit, per unit}.
var knownMetricUnits = map[string][2]string{
	"system.cpu.user":                            {"percent", ""},
	"system.cpu.system":                          {"percent", ""},
	"system.cpu.iowait":                          {"percent", ""},
	"system.cpu.idle":                            {"percent", ""},
	"system.cpu.stolen":                          {"percent", ""},
	"system.cpu.guest":                           {"percent", ""},
	"system.cpu.interrupt":                       {"percent", ""},
	"system.cpu.nice":                            {"percent", ""},
	"system.mem.total":                           {"mebibyte", ""},
	"system.mem.used":                            {"mebibyte", ""},
	"system.mem.free":                            {"mebibyte", ""},
	"system.mem.usable":                          {"mebibyte", ""},
	"system.mem.cached":                          {"mebibyte", ""},
	"system.mem.buffered":                        {"mebibyte", ""},
	"system.mem.shared":                          {"mebibyte", ""},
	"system.mem.slab":                            {"mebibyte", ""},
	"system.mem.pct_usable":                      {"fraction", ""},
	"system.swap.total":                          {"mebibyte", ""},
	"system.swap.used":                           {"mebibyte", ""},
	"system.swap.free":                           {"mebibyte", ""},
	"system.disk.total":                          {"kibibyte", ""},
	"system.disk.used":                           {"kibibyte", ""},
	"system.disk.free":                           {"kibibyte", ""},
	"system.disk.in_use":                         {"fraction", ""},
	"system.net.bytes_rcvd":                      {"byte", "second"},
	"system.net.bytes_sent":                      {"byte", "second"},
	"system.net.packets_in.count":                {"packet", "second"},
	"system.net.packets_out.count":               {"packet", "second"},
	"system.io.r_s":                              {"request", "second"},
	"system.io.w_s":                              {"request", "second"},
	"system.io.rkb_s":                            {"kibibyte", "second"},
	"system.io.wkb_s":                            {"kibibyte", "second"},
	"system.io.util":                             {"percent", ""},
	"system.uptime":                              {"second", ""},
	"system.load.norm.1":                         {"fraction", ""},
	"system.load.norm.5":                         {"fraction", ""},
	"system.load.norm.15":                        {"fraction", ""},
	"runtime.node.heap.total_heap_size":          {"byte", ""},
	"runtime.node.heap.used_heap_size":           {"byte", ""},
	"runtime.node.heap.total_physical_size":      {"byte", ""},
	"runtime.node.heap.heap_size_limit":          {"byte", ""},
	"runtime.node.mem.rss":                       {"byte", ""},
	"runtime.node.mem.heap_total":                {"byte", ""},
	"runtime.node.mem.heap_used":                 {"byte", ""},
	"runtime.node.mem.external":                  {"byte", ""},
	"runtime.node.cpu.user":                      {"percent", ""},
	"runtime.node.cpu.system":                    {"percent", ""},
	"runtime.node.cpu.total":                     {"percent", ""},
	"runtime.node.event_loop.delay.avg":          {"nanosecond", ""},
	"runtime.node.event_loop.delay.max":          {"nanosecond", ""},
	"runtime.node.event_loop.delay.95percentile": {"nanosecond", ""},
}

// metricUnits returns the unit and per-unit of a metric: the one sent by the agent, else a
// well-known one.
func metricUnits(metric, sent string) (string, string) {
	if sent != "" {
		return sent, ""
	}
	if u, ok := knownMetricUnits[metric]; ok {
		return u[0], u[1]
	}
	if strings.HasPrefix(metric, "trace.") {
		switch {
		case strings.HasSuffix(metric, ".hits"):
			return "hit", ""
		case strings.HasSuffix(metric, ".errors"):
			return "error", ""
		case strings.HasSuffix(metric, ".duration"):
			return "second", ""
		}
		return "second", ""
	}
	return "", ""
}

// resolveUnits replaces the units of query results by "unit" or "unit/per" names, so they
// survive formula evaluation.
func resolveUnits(metric string, res *store.MetricQueryResult) {
	for _, sr := range res.Series {
		unit, per := metricUnits(metric, sr.Unit)
		if per != "" {
			unit += "/" + per
		}
		sr.Unit = unit
	}
}

// unitPair renders a resolved unit name as the [unit, per unit] pair of the query API.
func unitPair(resolved string) any {
	unit, per, _ := strings.Cut(resolved, "/")
	u := unitObject(unit)
	if u == nil {
		return nil
	}
	return []any{u, unitObject(per)}
}
