// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
)

// MarshableMetric sole purpose is to provide a marshable reprensentation of a
// metric
type MarshableMetric struct {
	metric metric
}

// MarshalJSON returns a json representation of the current `metric`
func (mm MarshableMetric) MarshalJSON() ([]byte, error) {
	metric := mm.metric
	base := metric.base()
	return json.Marshal(struct {
		Name  string
		Type  string
		Tags  []string `json:",omitempty"`
		Opts  []string
		Value int64
	}{
		Name:  base.name,
		Type:  fmt.Sprintf("%T", metric),
		Tags:  sets.List(base.tags),
		Opts:  sets.List(base.opts),
		Value: base.Get(),
	})
}

// Handler is meant to be used in conjuntion with a HTTP server for exposing the
// state of all metrics currently tracked by this library
func Handler(w http.ResponseWriter, req *http.Request) {
	metrics := globalRegistry.GetMetrics()

	// sort entries by name it easier to read the output
	sort.Slice(metrics, func(i, j int) bool {
		return metrics[i].base().name < metrics[j].base().name
	})

	marshableMetrics := make([]MarshableMetric, len(metrics))
	for i, m := range metrics {
		marshableMetrics[i] = MarshableMetric{m}
	}

	utils.WriteAsJSON(w, marshableMetrics)
}
