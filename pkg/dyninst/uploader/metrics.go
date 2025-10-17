// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package uploader

import "sync/atomic"

// Metrics holds the statistics for an uploader.
type Metrics struct {
	// BatchesSent is the number of batches successfully sent.
	BatchesSent atomic.Int64
	// BytesSent is the total number of bytes successfully sent.
	BytesSent atomic.Int64
	// ItemsSent is the total number of items successfully sent.
	ItemsSent atomic.Int64
	// Errors is the number of errors encountered during upload attempts.
	Errors atomic.Int64
}

// Stats returns a map of the current metrics.
func (m *Metrics) Stats() map[string]int64 {
	return map[string]int64{
		"batches_sent": m.BatchesSent.Load(),
		"bytes_sent":   m.BytesSent.Load(),
		"items_sent":   m.ItemsSent.Load(),
		"errors":       m.Errors.Load(),
	}
}
