// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

const detectDigestFileName = "detect_digests.jsonl"

// detectDigest captures the output and input summary of a single Detect() call.
// Primary comparison is on anomaly output; input hash is secondary.
type detectDigest struct {
	DetectorName        string   `json:"detector"`
	DataTime            int64    `json:"data_time"`
	AnomalyCount        int      `json:"anomaly_count"`
	AnomalyFingerprints []string `json:"anomaly_fingerprints,omitempty"`
	InputHash           uint64   `json:"input_hash"`
	ReadCount           int      `json:"read_count"`
	PointCount          int      `json:"point_count"`
}

// anomalyFingerprint identifies an anomaly by source and time. DetectorName is
// already recorded on the enclosing digest; display text is not identity.
func anomalyFingerprint(a observerdef.Anomaly) string {
	return fmt.Sprintf("%s|%d", a.Source.Key(), a.Timestamp)
}
