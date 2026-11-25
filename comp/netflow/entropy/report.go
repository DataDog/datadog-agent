// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package entropy

import (
	"math"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
)

const metricPrefix = "datadog.netflow.entropy."

// ndmDeviceResourcePrefix makes the backend resolve the exporter to its NDM device and add the device's tags.
const ndmDeviceResourcePrefix = "dd.internal.resource:ndm_device:"

// Submit sends the entropy of valid windows, tagged by interface, and counts.
// Drops are tagged by exporter like the aggregator's sequence metrics. No metric has peer tags.
// Entropy is in bits across peers, weighted by bytes, packets or records. normalized_* is divided by log2(peers), 0 to 1.
func (r Report) Submit(s sender.Sender) {
	windowsByStatus := make(map[qualityStatus]int)
	for _, w := range r.Windows {
		windowsByStatus[w.Status]++
		if w.Status != statusValid {
			continue
		}
		tags := []string{
			"device_namespace:" + w.Namespace,
			"exporter_ip:" + w.Exporter.String(),
			"interface_index:" + strconv.FormatUint(uint64(w.InterfaceIndex), 10),
			"observation_point:" + string(w.observationPoint),
			ndmDeviceResourcePrefix + w.Namespace + ":" + w.Exporter.String(),
		}
		// stamped at the window end, so windows closed in the same flush don't overwrite each other
		ts := float64(w.End.Unix())
		maxEntropy := math.Log2(float64(w.DistinctCount))
		for _, e := range [...]struct {
			name  string
			value float64
		}{
			{"byte_weighted", w.ByteEntropy},
			{"packet_weighted", w.PacketEntropy},
			{"record_weighted", w.RecordEntropy},
		} {
			_ = s.GaugeWithTimestamp(metricPrefix+e.name, e.value, "", tags, ts) // only fails for ts <= 0
			// undefined below 2 peers
			if w.DistinctCount >= 2 {
				_ = s.GaugeWithTimestamp(metricPrefix+"normalized_"+e.name, e.value/maxEntropy, "", tags, ts)
			}
		}
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
