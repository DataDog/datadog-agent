// +build !linux

package util

import (
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// SetContainerSource is only implemented on Linux
func SetContainerSource(name string) {
	return
}

// GetContainers is only implemented on Linux
func GetContainers() ([]*containers.Container, error) {
	return nil, ErrNotImplemented
}

// ExtractContainerRateMetric extracts relevant rate values from a container list
// for later reuse, while reducing memory usage to only the needed fields
func ExtractContainerRateMetric(containers []*containers.Container) map[string]ContainerRateMetrics {
	return nil
}
