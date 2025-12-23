// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"sync"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// MemoryConsumer is a simple anomaly consumer that accumulates events in memory.
// It serves as an example implementation and for testing.
type MemoryConsumer struct {
	mu        sync.Mutex
	anomalies []observer.AnomalyOutput
}

// Name returns the consumer name.
func (m *MemoryConsumer) Name() string {
	return "memory_consumer"
}

// Consume adds an anomaly to the in-memory list.
func (m *MemoryConsumer) Consume(anomaly observer.AnomalyOutput) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.anomalies = append(m.anomalies, anomaly)
}

// Report logs all accumulated anomalies and clears the list.
func (m *MemoryConsumer) Report() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.anomalies) == 0 {
		return
	}

	fmt.Printf("[observer] Reporting %d anomalies:\n", len(m.anomalies))
	for i, a := range m.anomalies {
		fmt.Printf("  [%d] %s: %s (tags: %v)\n", i+1, a.Title, a.Description, a.Tags)
	}

	m.anomalies = nil
}

// GetAnomalies returns a copy of accumulated anomalies (for testing).
func (m *MemoryConsumer) GetAnomalies() []observer.AnomalyOutput {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]observer.AnomalyOutput, len(m.anomalies))
	copy(result, m.anomalies)
	return result
}
