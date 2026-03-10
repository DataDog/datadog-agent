// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"expvar"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	// LogsMemoryBudgetEnabled tracks whether the logs memory budget is enabled.
	LogsMemoryBudgetEnabled = expvar.Int{}
	// LogsMemoryBudgetBytesInUse tracks the total number of bytes currently held by the budget.
	LogsMemoryBudgetBytesInUse = expvar.Int{}
	// LogsMemoryBudgetMaxBytes tracks the configured maximum number of bytes allowed by the budget.
	LogsMemoryBudgetMaxBytes = expvar.Int{}
	// LogsMemoryBudgetOverflowCount tracks the number of budget overflow events.
	LogsMemoryBudgetOverflowCount = expvar.Int{}

	// TlmLogsMemoryBudgetBytesInUse tracks current budget usage by component.
	TlmLogsMemoryBudgetBytesInUse = telemetry.NewGauge("logs_memory_budget", "bytes_in_use", []string{"component"}, "Gauge of the number of bytes currently reserved in the logs memory budget")
	// TlmLogsMemoryBudgetEnabled tracks whether the budget is enabled.
	TlmLogsMemoryBudgetEnabled = telemetry.NewGauge("logs_memory_budget", "enabled", nil, "Gauge of whether the logs memory budget is enabled")
	// TlmLogsMemoryBudgetMaxBytes tracks the configured maximum bytes.
	TlmLogsMemoryBudgetMaxBytes = telemetry.NewGauge("logs_memory_budget", "max_bytes", nil, "Gauge of the configured byte limit for the logs memory budget")
	// TlmLogsMemoryBudgetOverflows tracks overflow events by component and operation.
	TlmLogsMemoryBudgetOverflows = telemetry.NewCounter("logs_memory_budget", "overflows", []string{"component", "operation"}, "Count of logs memory budget overflow events")
)
