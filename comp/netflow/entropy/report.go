// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package entropy

import (
	"math"
	"strconv"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const metricPrefix = "datadog.netflow.entropy."

// Submit logs valid windows and sends counts. Drops are tagged by exporter like the aggregator's
// sequence metrics; no metric has interface or peer tags.
func (r Report) Submit(s sender.Sender, logger log.Component) {
	windowsByStatus := make(map[qualityStatus]int)
	for _, w := range r.Windows {
		windowsByStatus[w.Status]++
		if w.Status != statusValid {
			continue
		}
		normalized := "none"
		// ByteEntropy / log2(DistinctCount), undefined below 2 peers
		if w.DistinctCount >= 2 {
			normalized = strconv.FormatFloat(w.ByteEntropy/math.Log2(float64(w.DistinctCount)), 'f', 4, 64)
		}
		logger.Infof("NetFlow interface entropy (shadow): namespace=%s exporter_ip=%s interface_index=%d observation_point=%s "+
			"window_start=%d window_end=%d entropy_bytes=%.4f normalized_entropy_bytes=%s entropy_packets=%.4f entropy_records=%.4f "+
			"total_weight=%d total_packets=%d record_count=%d distinct_count=%d agent_version=%s",
			w.Namespace, w.Exporter, w.InterfaceIndex, w.observationPoint,
			w.Start.Unix(), w.End.Unix(), w.ByteEntropy, normalized, w.PacketEntropy, w.RecordEntropy,
			w.TotalWeight, w.TotalPackets, w.RecordCount, w.DistinctCount, version.AgentVersion)
	}
	for status, n := range windowsByStatus {
		s.Count(metricPrefix+"windows", float64(n), "", []string{"quality_status:" + string(status)})
	}
	// counted per interface side of a record (ingress, egress), so up to 2 per record
	for k, n := range r.Dropped {
		tags := []string{"reason:" + string(k.Reason), "device_namespace:" + k.Namespace}
		if k.Exporter.IsValid() {
			tags = append(tags, "exporter_ip:"+k.Exporter.String())
		}
		s.Count(metricPrefix+"interface_records_dropped", float64(n), "", tags)
	}
	s.Gauge(metricPrefix+"state_entries", float64(r.StateEntries), "", nil)
}
