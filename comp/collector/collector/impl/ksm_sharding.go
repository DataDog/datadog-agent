// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectorimpl

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/kubestatemetrics/sharding"
)

func ksmShardingSnapshots(checks []check.Check) []sharding.CheckSnapshot {
	snapshots := make([]sharding.CheckSnapshot, 0)
	for _, ch := range checks {
		if inspector, ok := check.As[sharding.Inspector](ch); ok {
			snapshots = append(snapshots, inspector.KSMShardingSnapshot())
		}
	}
	slices.SortFunc(snapshots, func(a, b sharding.CheckSnapshot) int {
		return strings.Compare(a.CheckID, b.CheckID)
	})
	return snapshots
}

func (c *collectorImpl) writeKSMSharding(w http.ResponseWriter, _ *http.Request) {
	data, err := json.Marshal(ksmShardingSnapshots(c.GetChecks()))
	if err != nil {
		http.Error(w, "Unable to encode KSM sharding inventory", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}
