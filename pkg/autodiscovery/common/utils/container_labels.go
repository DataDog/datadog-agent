package utils

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

const (
	containerAnnotationPrefix = "com.datadoghq.ad."
)

// ExtractCheckNamesFromContainerLabels returns check names from a map of
// container annotations. It prefers annotations v2 where available, otherwise
// falling back to v1.
func ExtractCheckNamesFromContainerLabels(labels map[string]string) ([]string, error) {
	return extractCheckNamesFromMap(labels, containerAnnotationPrefix, "")
}

// ExtractTemplatesFromContainerLabels looks for autodiscovery configurations
// in a map of labels and returns them if found. In order of priority, it
// prefers annotations v2, and then v1.
func ExtractTemplatesFromContainerLabels(entityName string, labels map[string]string) ([]integration.Config, []error) {
	return extractTemplatesFromMapWithV2(entityName, labels, containerAnnotationPrefix, "")
}
