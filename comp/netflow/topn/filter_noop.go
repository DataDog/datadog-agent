// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package topn defines business logic for filtering NetFlow records to the Top "N" occurrences.
package topn

import "github.com/DataDog/datadog-agent/comp/netflow/common"

// NoopFilter implements the FlowFlushFilter interface to return the full list of flows with now filtering.
type NoopFilter struct {
}

// Filter implements the FlowFlushFilter interface to return the full list of flows without any filtering.
func (n NoopFilter) Filter(_ common.FlushContext, flows []*common.Flow) []*common.Flow {
	return flows
}
