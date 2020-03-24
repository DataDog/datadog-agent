package util

import "github.com/DataDog/datadog-agent/pkg/util/containers/metrics"

// ContainerRateMetrics holds previous values for a container,
// in order to compute rates
type ContainerRateMetrics struct {
	CPU        *metrics.ContainerCPUStats
	IO         *metrics.ContainerIOStats
	NetworkSum *metrics.InterfaceNetStats
	Network    metrics.ContainerNetStats
}

// NullContainerRates can be safely used for containers that have no
// previours rate values stored (new containers)
var NullContainerRates = ContainerRateMetrics{
	CPU:        &metrics.ContainerCPUStats{},
	IO:         &metrics.ContainerIOStats{},
	NetworkSum: &metrics.InterfaceNetStats{},
	Network:    metrics.ContainerNetStats{},
}
